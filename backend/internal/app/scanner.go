package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultCodexUsageURL = "https://chatgpt.com/backend-api/wham/usage"
	defaultCodexUA       = "codex_cli_rs/0.76.0 (Debian 13.0.0; x86_64) WindowsTerminal"
	defaultRetryAttempts = 3
	defaultRetryBackoff  = 250 * time.Millisecond
	maxErrorBodyPreview  = 240
	defaultUnknownCPUWorkers = 20
	workersPerDetectedCPU    = 20
)

type scanFile struct {
	absPath   string
	relPath   string
	baseName  string
	sortKey   string
}

func normalizeSelectedID(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	normalized := strings.ReplaceAll(trimmed, "\\", "/")
	normalized = strings.TrimPrefix(normalized, "./")
	normalized = strings.TrimPrefix(normalized, "/")
	normalized = pathCleanSlash(normalized)
	return strings.ToLower(normalized)
}

func pathCleanSlash(value string) string {
	parts := strings.Split(value, "/")
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			if len(filtered) > 0 {
				filtered = filtered[:len(filtered)-1]
			}
			continue
		}
		filtered = append(filtered, part)
	}
	return strings.Join(filtered, "/")
}

type authFile struct {
	Type        string `json:"type"`
	Email       string `json:"email"`
	Expired     string `json:"expired"`
	LastRefresh string `json:"last_refresh"`
	Disabled    bool   `json:"disabled"`
	IDToken     string `json:"id_token"`
	AccessToken string `json:"access_token"`
	AccountID   string `json:"account_id"`
}

type codexClaimsInfo struct {
	PlanType  string
	AccountID string
}

type scanOptions struct {
	DirectoryPath    string
	DirectoryName    string
	FullValueUSD     float64
	AutoConcurrency  bool
	RequestedWorkers int
	SelectedIDs      map[string]struct{}
	Progress         func(done, total int)
}

func ScanDirectory(ctx context.Context, opts scanOptions) (ScanSnapshot, error) {
	if strings.TrimSpace(opts.DirectoryPath) == "" {
		return ScanSnapshot{}, fmt.Errorf("directory path is empty")
	}
	files, err := collectScanFiles(opts.DirectoryPath, opts.SelectedIDs)
	if err != nil {
		return ScanSnapshot{}, fmt.Errorf("read directory failed: %w", err)
	}
	return scanPreparedFiles(ctx, files, opts), nil
}

func ScanSelectedFiles(ctx context.Context, opts scanOptions, relativePaths []string) (ScanSnapshot, error) {
	if strings.TrimSpace(opts.DirectoryPath) == "" {
		return ScanSnapshot{}, fmt.Errorf("directory path is empty")
	}
	files := make([]scanFile, 0, len(relativePaths))
	seen := make(map[string]struct{}, len(relativePaths))
	for _, item := range relativePaths {
		raw := strings.TrimSpace(strings.ReplaceAll(item, "\\", "/"))
		raw = strings.TrimPrefix(raw, "./")
		raw = strings.TrimPrefix(raw, "/")
		raw = pathCleanSlash(raw)
		normalized := normalizeSelectedID(raw)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		candidatePath := filepath.Join(opts.DirectoryPath, filepath.FromSlash(raw))
		info, err := os.Stat(candidatePath)
		if err != nil || info == nil || info.IsDir() || !strings.HasSuffix(strings.ToLower(info.Name()), ".json") {
			continue
		}
		seen[normalized] = struct{}{}
		files = append(files, scanFile{absPath: candidatePath, relPath: filepath.ToSlash(raw), baseName: info.Name(), sortKey: strings.ToLower(raw)})
	}
	if len(files) == 0 {
		return ScanSnapshot{}, nil
	}
	return scanPreparedFiles(ctx, files, opts), nil
}

func scanPreparedFiles(ctx context.Context, files []scanFile, opts scanOptions) ScanSnapshot {
	sort.Slice(files, func(i, j int) bool {
		return files[i].sortKey < files[j].sortKey
	})

	logicalCPU := runtime.NumCPU()
	recommended := DetectRecommendedConcurrency(logicalCPU)
	workers := recommended
	if opts.AutoConcurrency {
		workers = DetectEffectiveAutoConcurrency(len(files), logicalCPU)
	} else if opts.RequestedWorkers > 0 {
		workers = opts.RequestedWorkers
	}
	workers = clampWorkers(len(files), workers)

	start := time.Now()
	rows := inspectAll(ctx, files, opts.FullValueUSD, workers, opts.Progress)
	snapshot := ScanSnapshot{
		Directory:              opts.DirectoryName,
		DirectoryPath:          opts.DirectoryPath,
		ScannedAt:              time.Now().Format(time.RFC3339),
		DurationMs:             time.Since(start).Milliseconds(),
		FullValueUSD:           round2(opts.FullValueUSD),
		AutoConcurrency:        opts.AutoConcurrency,
		ConcurrencyUsed:        workers,
		RecommendedConcurrency: recommended,
		LogicalCPU:             logicalCPU,
		Accounts:               rows,
	}
	snapshot.Summary = buildSummary(rows)
	return snapshot
}

func collectScanFiles(root string, selectedIDs map[string]struct{}) ([]scanFile, error) {
	if len(selectedIDs) > 0 {
		direct := make([]scanFile, 0, len(selectedIDs))
		seen := make(map[string]struct{}, len(selectedIDs))
		for selectedID := range selectedIDs {
			candidatePath := filepath.Join(root, filepath.FromSlash(selectedID))
			info, err := os.Stat(candidatePath)
			if err != nil || info == nil || info.IsDir() || !strings.HasSuffix(strings.ToLower(info.Name()), ".json") {
				continue
			}
			rel, relErr := filepath.Rel(root, candidatePath)
			if relErr != nil {
				rel = info.Name()
			}
			rel = filepath.ToSlash(rel)
			normalizedRel := normalizeSelectedID(rel)
			if _, ok := seen[normalizedRel]; ok {
				continue
			}
			seen[normalizedRel] = struct{}{}
			direct = append(direct, scanFile{absPath: candidatePath, relPath: rel, baseName: info.Name(), sortKey: strings.ToLower(rel)})
		}
		if len(direct) > 0 {
			return direct, nil
		}
	}

	files := make([]scanFile, 0, 128)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d == nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".json") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = d.Name()
		}
		rel = filepath.ToSlash(rel)
		if len(selectedIDs) > 0 {
			normalizedName := normalizeSelectedID(d.Name())
			normalizedRel := normalizeSelectedID(rel)
			if _, ok := selectedIDs[normalizedName]; !ok {
				if _, ok = selectedIDs[normalizedRel]; !ok {
					return nil
				}
			}
		}
		files = append(files, scanFile{absPath: path, relPath: rel, baseName: d.Name(), sortKey: strings.ToLower(rel)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func DetectRecommendedConcurrency(logicalCPU int) int {
	if logicalCPU <= 0 {
		return defaultUnknownCPUWorkers
	}
	return logicalCPU * workersPerDetectedCPU
}

func RecommendConcurrency(taskCount int, logicalCPU int) int {
	recommended := DetectRecommendedConcurrency(logicalCPU)
	if taskCount > 0 && recommended > taskCount {
		recommended = taskCount
	}
	return recommended
}

func DetectEffectiveAutoConcurrency(taskCount int, logicalCPU int) int {
	workers := DetectRecommendedConcurrency(logicalCPU)
	return clampWorkers(taskCount, workers)
}

func inspectAll(ctx context.Context, files []scanFile, fullValueUSD float64, workerCount int, progressFn func(done, total int)) []AccountRecord {
	if len(files) == 0 {
		return nil
	}
	rows := make([]AccountRecord, len(files))
	jobs := make(chan int, minInt(len(files), maxInt(32, workerCount*2)))
	var wg sync.WaitGroup
	client := newHTTPClient(30*time.Second, workerCount)
	var doneCount atomic.Int64
	progressStep := progressUpdateStep(len(files))

	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}
				file := files[index]
				row, err := inspectAuthFileLive(ctx, file, fullValueUSD, client)
				if err != nil {
					display := file.relPath
					if display == "" {
						display = file.baseName
					}
					rows[index] = AccountRecord{ID: display, File: display, Email: display, Plan: "free", Status: AccountStatusDisabled, Note: err.Error()}
				} else {
					rows[index] = row
				}
				if progressFn != nil {
					done := int(doneCount.Add(1))
					if done == 1 || done == len(files) || done%progressStep == 0 {
						progressFn(done, len(files))
					}
				}
			}
		}()
	}
	for index := range files {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	return rows
}

func newHTTPClient(timeout time.Duration, workerCount int) *http.Client {
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok || transport == nil {
		return &http.Client{Timeout: timeout}
	}
	clone := transport.Clone()
	if workerCount < 32 {
		workerCount = 32
	}
	clone.MaxIdleConns = workerCount * 2
	clone.MaxIdleConnsPerHost = workerCount
	clone.MaxConnsPerHost = workerCount
	clone.IdleConnTimeout = 90 * time.Second
	clone.ForceAttemptHTTP2 = true
	return &http.Client{Timeout: timeout, Transport: clone}
}

func inspectAuthFileLive(ctx context.Context, file scanFile, fullValueUSD float64, client *http.Client) (AccountRecord, error) {
	basic, err := readAuthPayload(file.absPath)
	if err != nil {
		return AccountRecord{}, err
	}
	claimsInfo := extractCodexClaimsInfo(basic)
	plan := claimsInfo.PlanType
	if plan == "" {
		plan = "free"
	}
	displayFile := file.relPath
	if displayFile == "" {
		displayFile = file.baseName
	}
	row := AccountRecord{
		ID:          displayFile,
		File:        displayFile,
		Email:       fallbackString(strings.TrimSpace(basic.Email), displayFile),
		Plan:        plan,
		Disabled:    basic.Disabled,
		LastRefresh: strings.TrimSpace(basic.LastRefresh),
		ExpiredAt:   strings.TrimSpace(basic.Expired),
		Status:      AccountStatusDepleted,
	}

	if strings.ToLower(strings.TrimSpace(basic.Type)) != "codex" {
		row.Note = "当前仅支持 codex 认证文件"
		row.Status = AccountStatusDisabled
		return row, nil
	}
	if strings.TrimSpace(basic.AccessToken) == "" {
		row.Note = "缺少 access_token"
		row.Status = AccountStatusDisabled
		return row, nil
	}
	accountID := fallbackString(strings.TrimSpace(basic.AccountID), claimsInfo.AccountID)
	if accountID == "" {
		row.Note = "缺少 account_id"
		row.Status = AccountStatusDisabled
		return row, nil
	}

	payload, statusCode, bodyText, err := requestCodexUsage(ctx, client, basic.AccessToken, accountID)
	row.StatusCode = statusCode
	if err != nil {
		if statusCode > 0 {
			row.Note = formatStatusNote(statusCode, bodyText)
			row.Status = deriveStatus(row, nil)
			return row, nil
		}
		return row, err
	}

	if planType, ok := payload["plan_type"].(string); ok && strings.TrimSpace(planType) != "" {
		row.Plan = strings.TrimSpace(planType)
	}
	windows, percent, resetDate := extractCodexWindows(payload)
	row.Windows = windows
	row.ResetDate = resetDate
	if percent != nil {
		row.QuotaPercent = clampPercent(*percent)
		row.USDValue = round2(fullValueUSD * row.QuotaPercent / 100)
	}
	row.Status = deriveStatus(row, percent)
	if percent == nil {
		row.Note = fallbackString(row.Note, "实时接口未返回可计价窗口")
	}
	return row, nil
}

func readAuthPayload(path string) (authFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return authFile{}, fmt.Errorf("读取失败: %w", err)
	}
	var basic authFile
	if err = json.Unmarshal(raw, &basic); err != nil {
		return authFile{}, fmt.Errorf("JSON 无效: %w", err)
	}
	return basic, nil
}

func extractCodexClaimsInfo(basic authFile) codexClaimsInfo {
	if strings.TrimSpace(basic.IDToken) == "" {
		return codexClaimsInfo{}
	}
	claims, err := parseJWTToken(strings.TrimSpace(basic.IDToken))
	if err != nil || claims == nil {
		return codexClaimsInfo{}
	}
	return codexClaimsInfo{
		PlanType:  strings.TrimSpace(claims.CodexAuthInfo.ChatgptPlanType),
		AccountID: strings.TrimSpace(claims.CodexAuthInfo.ChatgptAccountID),
	}
}

func requestCodexUsage(ctx context.Context, client *http.Client, accessToken, accountID string) (map[string]any, int, string, error) {
	var lastErr error
	var lastStatus int
	var lastBody string
	for attempt := 1; attempt <= defaultRetryAttempts; attempt++ {
		payload, statusCode, bodyText, retryable, err := requestCodexUsageOnce(ctx, client, accessToken, accountID)
		if err == nil {
			return payload, statusCode, bodyText, nil
		}
		lastErr = err
		lastStatus = statusCode
		lastBody = bodyText
		if !retryable || attempt == defaultRetryAttempts {
			break
		}
		if err = waitForRetry(ctx, time.Duration(attempt)*defaultRetryBackoff); err != nil {
			break
		}
	}
	return nil, lastStatus, lastBody, lastErr
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func progressUpdateStep(total int) int {
	switch {
	case total <= 1_000:
		return 1
	case total <= 10_000:
		return 10
	case total <= 100_000:
		return 100
	default:
		return 500
	}
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func requestCodexUsageOnce(ctx context.Context, client *http.Client, accessToken, accountID string) (map[string]any, int, string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, defaultCodexUsageURL, nil)
	if err != nil {
		return nil, 0, "", false, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	req.Header.Set("Chatgpt-Account-Id", strings.TrimSpace(accountID))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", defaultCodexUA)

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, "", isRetryableRequestError(err), fmt.Errorf("请求额度失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	bodyText := strings.TrimSpace(string(body))
	if err != nil {
		return nil, resp.StatusCode, bodyText, isRetryableStatusCode(resp.StatusCode), fmt.Errorf("读取额度响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, bodyText, isRetryableStatusCode(resp.StatusCode), fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	payload, err := parseJSONMap(body)
	if err != nil {
		return nil, resp.StatusCode, bodyText, false, fmt.Errorf("解析额度响应失败: %w", err)
	}
	return payload, resp.StatusCode, bodyText, false, nil
}

func parseJSONMap(raw []byte) (map[string]any, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("响应体为空")
	}
	var payload map[string]any
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, fmt.Errorf("响应体不是 JSON 对象")
	}
	return payload, nil
}

func extractCodexWindows(payload map[string]any) ([]CodexUsageWindow, *float64, string) {
	windows := make([]CodexUsageWindow, 0, 8)
	if rateLimit, ok := firstMap(payload, "rate_limit", "rateLimit"); ok {
		windows = appendRateLimitWindows(windows, rateLimit, "")
	}
	if rateLimit, ok := firstMap(payload, "code_review_rate_limit", "codeReviewRateLimit"); ok {
		windows = appendRateLimitWindows(windows, rateLimit, "code-review")
	}
	if items, ok := firstSlice(payload, "additional_rate_limits", "additionalRateLimits"); ok {
		for index, item := range items {
			itemMap, okMap := item.(map[string]any)
			if !okMap || itemMap == nil {
				continue
			}
			rateLimit, okRate := firstMap(itemMap, "rate_limit", "rateLimit")
			if !okRate {
				continue
			}
			name := strings.TrimSpace(firstString(itemMap, "limit_name", "limitName", "metered_feature", "meteredFeature"))
			if name == "" {
				name = fmt.Sprintf("additional-%d", index+1)
			}
			windows = appendRateLimitWindows(windows, rateLimit, name)
		}
	}

	var chosen *float64
	var soonest *time.Time
	for _, window := range windows {
		if window.RemainingPercent != nil {
			if chosen == nil || *window.RemainingPercent < *chosen {
				chosen = floatPtr(*window.RemainingPercent)
			}
		}
		if window.ResetAt != nil && *window.ResetAt > 0 {
			ts := time.Unix(*window.ResetAt, 0).UTC()
			if soonest == nil || ts.Before(*soonest) {
				copied := ts
				soonest = &copied
			}
		}
	}
	resetDate := ""
	if soonest != nil {
		resetDate = soonest.Format(time.RFC3339)
	}
	return windows, chosen, resetDate
}

func appendRateLimitWindows(dst []CodexUsageWindow, rateLimit map[string]any, prefix string) []CodexUsageWindow {
	allowed, _ := firstBool(rateLimit, "allowed")
	limitReached, _ := firstBool(rateLimit, "limit_reached", "limitReached")
	for _, item := range []struct {
		label string
		keys  []string
	}{
		{label: buildWindowLabel(prefix, "primary"), keys: []string{"primary_window", "primaryWindow"}},
		{label: buildWindowLabel(prefix, "secondary"), keys: []string{"secondary_window", "secondaryWindow"}},
	} {
		windowMap, ok := firstMap(rateLimit, item.keys...)
		if !ok {
			continue
		}
		window := CodexUsageWindow{Label: item.label}
		if remaining, okRemaining := firstFloat(windowMap, "remaining_percent", "remainingPercent"); okRemaining {
			window.RemainingPercent = floatPtr(clampPercent(remaining))
			window.UsedPercent = floatPtr(clampPercent(100 - remaining))
		} else if used, okUsed := firstFloat(windowMap, "used_percent", "usedPercent"); okUsed {
			window.UsedPercent = floatPtr(round2(used))
			window.RemainingPercent = floatPtr(clampPercent(100 - used))
		} else if limitReached || !allowed {
			window.RemainingPercent = floatPtr(0)
		}
		if seconds, okSeconds := firstInt64(windowMap, "limit_window_seconds", "limitWindowSeconds"); okSeconds {
			window.LimitWindowSeconds = int64Ptr(seconds)
		}
		if seconds, okSeconds := firstInt64(windowMap, "reset_after_seconds", "resetAfterSeconds"); okSeconds {
			window.ResetAfterSeconds = int64Ptr(seconds)
		}
		if resetAt, okResetAt := firstInt64(windowMap, "reset_at", "resetAt"); okResetAt {
			window.ResetAt = int64Ptr(resetAt)
			window.ResetAtISO = time.Unix(resetAt, 0).UTC().Format(time.RFC3339)
		}
		dst = append(dst, window)
	}
	return dst
}

func buildWindowLabel(prefix, suffix string) string {
	suffix = strings.TrimSpace(suffix)
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return suffix
	}
	return prefix + "-" + suffix
}

func deriveStatus(row AccountRecord, percent *float64) AccountStatus {
	if row.Disabled || row.StatusCode == http.StatusUnauthorized || row.StatusCode == http.StatusForbidden {
		return AccountStatusDisabled
	}
	if row.ExpiredAt != "" {
		if ts, err := time.Parse(time.RFC3339, row.ExpiredAt); err == nil && time.Now().After(ts) {
			return AccountStatusExpired
		}
	}
	if percent == nil || *percent <= 0 {
		return AccountStatusDepleted
	}
	return AccountStatusNormal
}

func buildSummary(rows []AccountRecord) Summary {
	result := Summary{TotalAccounts: len(rows)}
	if len(rows) == 0 {
		return result
	}
	var sum float64
	var minSet bool
	now := time.Now()
	modifiedRecent := 0
	for _, row := range rows {
		if row.Note == "" || row.StatusCode == http.StatusOK {
			result.SuccessCount++
		} else {
			result.FailedCount++
		}
		sum += row.QuotaPercent
		result.TotalValueUSD += row.USDValue
		if !minSet || row.QuotaPercent < result.MinQuotaPercent {
			result.MinQuotaPercent = row.QuotaPercent
			minSet = true
		}
		if row.QuotaPercent > result.MaxQuotaPercent {
			result.MaxQuotaPercent = row.QuotaPercent
		}
		switch {
		case row.QuotaPercent >= 80:
			result.QuotaDistribution.Healthy++
		case row.QuotaPercent >= 40:
			result.QuotaDistribution.Medium++
		case row.QuotaPercent > 0:
			result.QuotaDistribution.Low++
		default:
			result.QuotaDistribution.Depleted++
		}
		if row.LastRefresh != "" {
			if ts, err := time.Parse(time.RFC3339, row.LastRefresh); err == nil && now.Sub(ts) <= 30*24*time.Hour {
				modifiedRecent++
			}
		}
	}
	result.TotalValueUSD = round2(result.TotalValueUSD)
	result.AverageQuotaPercent = round2(sum / float64(len(rows)))
	base := len(rows) - modifiedRecent
	if base <= 0 {
		if modifiedRecent > 0 {
			result.MonthlyGrowthPercent = 100
		}
	} else {
		result.MonthlyGrowthPercent = round2(float64(modifiedRecent) * 100 / float64(base))
	}
	return result
}

func clampWorkers(taskCount int, workers int) int {
	if taskCount <= 0 {
		return 0
	}
	if workers <= 0 {
		workers = 1
	}
	if workers > taskCount {
		return taskCount
	}
	return workers
}

func isRetryableRequestError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}
	return true
}

func isRetryableStatusCode(statusCode int) bool {
	switch statusCode {
	case http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return statusCode >= 500 && statusCode <= 599
}

func formatStatusNote(statusCode int, bodyText string) string {
	bodyText = strings.TrimSpace(bodyText)
	if bodyText == "" {
		return fmt.Sprintf("HTTP %d", statusCode)
	}
	return fmt.Sprintf("HTTP %d %s", statusCode, truncateText(bodyText, maxErrorBodyPreview))
}

func truncateText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

func round2(value float64) float64       { return math.Round(value*100) / 100 }
func clampPercent(value float64) float64 { return round2(math.Max(0, math.Min(100, value))) }
func floatPtr(value float64) *float64    { v := value; return &v }
func int64Ptr(value int64) *int64        { v := value; return &v }

func fallbackString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstString(source map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := source[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstFloat(source map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		if value, ok := anyToFloat(source[key]); ok {
			return value, true
		}
	}
	return 0, false
}

func firstInt64(source map[string]any, keys ...string) (int64, bool) {
	for _, key := range keys {
		if value, ok := anyToInt64(source[key]); ok {
			return value, true
		}
	}
	return 0, false
}

func firstBool(source map[string]any, keys ...string) (bool, bool) {
	for _, key := range keys {
		if value, ok := source[key].(bool); ok {
			return value, true
		}
	}
	return false, false
}

func firstMap(source map[string]any, keys ...string) (map[string]any, bool) {
	for _, key := range keys {
		if value, ok := source[key].(map[string]any); ok && value != nil {
			return value, true
		}
	}
	return nil, false
}

func firstSlice(source map[string]any, keys ...string) ([]any, bool) {
	for _, key := range keys {
		if value, ok := source[key].([]any); ok && value != nil {
			return value, true
		}
	}
	return nil, false
}

func anyToFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		trimmed := strings.TrimSpace(strings.TrimSuffix(typed, "%"))
		parsed, err := strconv.ParseFloat(trimmed, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func anyToInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return int64(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

type jwtClaims struct {
	CodexAuthInfo struct {
		ChatgptAccountID string `json:"chatgpt_account_id"`
		ChatgptPlanType  string `json:"chatgpt_plan_type"`
	} `json:"https://api.openai.com/auth"`
}

func parseJWTToken(token string) (*jwtClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT token format")
	}
	claimsData, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, err
	}
	var claims jwtClaims
	if err = json.Unmarshal(claimsData, &claims); err != nil {
		return nil, err
	}
	return &claims, nil
}

func base64URLDecode(data string) ([]byte, error) {
	switch len(data) % 4 {
	case 2:
		data += "=="
	case 3:
		data += "="
	}
	return base64.URLEncoding.DecodeString(data)
}
