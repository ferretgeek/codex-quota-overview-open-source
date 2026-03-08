package app

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	directoryListingCacheTTL = 10 * time.Second
	completedJobRetention    = 3 * time.Minute
	failedJobRetention       = 10 * time.Minute
	resultsDirName           = "results"
	previewAccountLimit      = 6
	maxAccountsPageSize      = 500
)

type cacheEntry struct {
	fullValue float64
	snapshot  ScanSnapshot
	createdAt time.Time
}

type Server struct {
	config ServerConfig
	mux    *http.ServeMux
	start  time.Time

	mu sync.RWMutex

	cache map[string]cacheEntry
	jobs  map[string]*ScanJob

	directoryListingCache   []DirectoryInfo
	directoryListingCacheAt time.Time
	importedDirectoryCounts map[string]int
}

func NewServer(config ServerConfig) *Server {
	if config.CacheTTL <= 0 {
		config.CacheTTL = 20 * time.Second
	}
	if config.DefaultPrice <= 0 {
		config.DefaultPrice = 7.5
	}
	s := &Server{
		config:                  config,
		mux:                     http.NewServeMux(),
		start:                   time.Now(),
		cache:                   make(map[string]cacheEntry),
		jobs:                    make(map[string]*ScanJob),
		importedDirectoryCounts: make(map[string]int),
	}
	s.registerRoutes()
	return s
}

func (s *Server) Config() ServerConfig  { return s.config }
func (s *Server) Handler() http.Handler { return s.cors(s.gzip(s.mux)) }

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/api/meta", s.handleMeta)
	s.mux.HandleFunc("/api/import-folder", s.handleImportFolder)
	s.mux.HandleFunc("/api/delete-directory", s.handleDeleteDirectory)
	s.mux.HandleFunc("/api/clear-imported-files", s.handleClearImportedFiles)
	s.mux.HandleFunc("/api/clear-stats", s.handleClearStats)
	s.mux.HandleFunc("/api/scan-job", s.handleScanJob)
	s.mux.HandleFunc("/api/refresh-job", s.handleRefreshJob)
	s.mux.HandleFunc("/api/job", s.handleGetJob)
	s.mux.HandleFunc("/api/accounts", s.handleAccountsPage)
	s.mux.HandleFunc("/api/scan", s.handleScan)
	s.mux.HandleFunc("/api/refresh", s.handleRefresh)
	s.mux.HandleFunc("/api/export.csv", s.handleExportCSV)
	s.mux.HandleFunc("/", s.handleStatic)
}

func cloneDirectoryInfos(items []DirectoryInfo) []DirectoryInfo {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]DirectoryInfo, len(items))
	copy(cloned, items)
	return cloned
}

func cloneAccountRecords(items []AccountRecord) []AccountRecord {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]AccountRecord, len(items))
	copy(cloned, items)
	return cloned
}

func normalizeDirectoryKey(path string) string {
	return filepath.Clean(strings.TrimSpace(path))
}

func (s *Server) pruneExpiredStateLocked(now time.Time) {
	for key, entry := range s.cache {
		if now.Sub(entry.createdAt) > s.config.CacheTTL {
			delete(s.cache, key)
		}
	}
	for jobID, job := range s.jobs {
		if job == nil {
			delete(s.jobs, jobID)
			continue
		}
		if job.Status == "running" {
			continue
		}
		finishedAt := job.finishedAtTime
		if finishedAt.IsZero() && strings.TrimSpace(job.FinishedAt) != "" {
			parsed, err := time.Parse(time.RFC3339, job.FinishedAt)
			if err == nil {
				finishedAt = parsed
				job.finishedAtTime = parsed
			}
		}
		if finishedAt.IsZero() {
			delete(s.jobs, jobID)
			continue
		}
		retention := completedJobRetention
		if job.Status == "failed" {
			retention = failedJobRetention
		}
		if now.Sub(finishedAt) > retention {
			delete(s.jobs, jobID)
		}
	}
}

func (s *Server) invalidateDirectoryListingLocked() {
	s.directoryListingCache = nil
	s.directoryListingCacheAt = time.Time{}
}

func (s *Server) invalidateDirectoryListing() {
	s.mu.Lock()
	s.invalidateDirectoryListingLocked()
	s.mu.Unlock()
}

func (s *Server) getCachedImportedDirectoryCount(path string) (int, bool) {
	key := normalizeDirectoryKey(path)
	s.mu.RLock()
	count, ok := s.importedDirectoryCounts[key]
	s.mu.RUnlock()
	return count, ok
}

func (s *Server) attachResultIDToCachedSnapshotLocked(resultID string, snapshot ScanSnapshot) {
	if resultID == "" {
		return
	}
	for key, entry := range s.cache {
		if entry.snapshot.DirectoryPath != snapshot.DirectoryPath {
			continue
		}
		if entry.snapshot.ScannedAt != snapshot.ScannedAt {
			continue
		}
		if len(entry.snapshot.Accounts) != len(snapshot.Accounts) {
			continue
		}
		entry.snapshot.ResultID = resultID
		s.cache[key] = entry
	}
}

func (s *Server) getCachedAccountsByResultID(resultID string) ([]AccountRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredStateLocked(time.Now())
	for _, entry := range s.cache {
		if entry.snapshot.ResultID != resultID || len(entry.snapshot.Accounts) == 0 {
			continue
		}
		return cloneAccountRecords(entry.snapshot.Accounts), true
	}
	return nil, false
}

func (s *Server) loadMergeBaseSnapshot(cacheKey string, resultID string) (ScanSnapshot, bool) {
	if existing, ok := s.getCachedSnapshot(cacheKey); ok && len(existing.Accounts) > 0 {
		return existing, true
	}
	if strings.TrimSpace(resultID) == "" {
		return ScanSnapshot{}, false
	}
	meta, err := s.loadPersistedSnapshot(strings.TrimSpace(resultID))
	if err != nil {
		return ScanSnapshot{}, false
	}
	accounts, err := s.loadPersistedAccounts(strings.TrimSpace(resultID))
	if err != nil {
		return ScanSnapshot{}, false
	}
	meta.Accounts = accounts
	meta.AccountsPartial = false
	meta.StoredAccountCount = len(accounts)
	return meta, true
}

func (s *Server) setImportedDirectoryCount(path string, count int) {
	key := normalizeDirectoryKey(path)
	s.mu.Lock()
	s.importedDirectoryCounts[key] = count
	s.invalidateDirectoryListingLocked()
	s.mu.Unlock()
}

func (s *Server) removeImportedDirectoryCount(path string) {
	key := normalizeDirectoryKey(path)
	s.mu.Lock()
	delete(s.importedDirectoryCounts, key)
	s.invalidateDirectoryListingLocked()
	s.mu.Unlock()
}

func (s *Server) clearImportedDirectoryCountsUnder(root string) {
	cleanRoot := normalizeDirectoryKey(root)
	s.mu.Lock()
	for key := range s.importedDirectoryCounts {
		if key == cleanRoot || strings.HasPrefix(key, cleanRoot+string(os.PathSeparator)) {
			delete(s.importedDirectoryCounts, key)
		}
	}
	s.invalidateDirectoryListingLocked()
	s.mu.Unlock()
}

func (s *Server) resultsRoot() string {
	return filepath.Join(s.config.AppRoot, resultsDirName)
}

func (s *Server) clearPersistedResults() error {
	root := s.resultsRoot()
	if err := os.RemoveAll(root); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func resultMetaPath(root, resultID string) string {
	return filepath.Join(root, resultID, "meta.json")
}

func resultAccountsPath(root, resultID string) string {
	return filepath.Join(root, resultID, "accounts.ndjson")
}

func buildResultID(snapshot ScanSnapshot) string {
	seed := fmt.Sprintf("%s|%s|%d", snapshot.DirectoryPath, snapshot.ScannedAt, len(snapshot.Accounts))
	hash := sha1.Sum([]byte(seed))
	return fmt.Sprintf("result-%d-%s", time.Now().UnixNano(), hex.EncodeToString(hash[:4]))
}

func buildPreviewAccounts(accounts []AccountRecord) []AccountRecord {
	if len(accounts) == 0 {
		return nil
	}
	preview := cloneAccountRecords(accounts)
	sort.Slice(preview, func(i, j int) bool {
		if preview[i].QuotaPercent == preview[j].QuotaPercent {
			return strings.ToLower(preview[i].File) < strings.ToLower(preview[j].File)
		}
		return preview[i].QuotaPercent < preview[j].QuotaPercent
	})
	if len(preview) > previewAccountLimit {
		preview = preview[:previewAccountLimit]
	}
	return cloneAccountRecords(preview)
}

func buildClientSnapshot(snapshot ScanSnapshot, resultID string) ScanSnapshot {
	client := snapshot
	client.ResultID = resultID
	client.StoredAccountCount = len(snapshot.Accounts)
	client.PreviewAccounts = buildPreviewAccounts(snapshot.Accounts)
	client.AccountsPartial = true
	client.Accounts = nil
	return client
}

func (s *Server) persistSnapshot(snapshot ScanSnapshot) (ScanSnapshot, error) {
	resultID := buildResultID(snapshot)
	root := s.resultsRoot()
	dir := filepath.Join(root, resultID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ScanSnapshot{}, err
	}
	accountsFile, err := os.Create(resultAccountsPath(root, resultID))
	if err != nil {
		return ScanSnapshot{}, err
	}
	writer := bufio.NewWriterSize(accountsFile, 1<<20)
	encoder := json.NewEncoder(writer)
	for _, account := range snapshot.Accounts {
		if err = encoder.Encode(account); err != nil {
			_ = accountsFile.Close()
			return ScanSnapshot{}, err
		}
	}
	if err = writer.Flush(); err != nil {
		_ = accountsFile.Close()
		return ScanSnapshot{}, err
	}
	if err = accountsFile.Close(); err != nil {
		return ScanSnapshot{}, err
	}
	clientSnapshot := buildClientSnapshot(snapshot, resultID)
	metaBytes, err := json.MarshalIndent(clientSnapshot, "", "  ")
	if err != nil {
		return ScanSnapshot{}, err
	}
	if err = os.WriteFile(resultMetaPath(root, resultID), metaBytes, 0o644); err != nil {
		return ScanSnapshot{}, err
	}
	return clientSnapshot, nil
}

func (s *Server) loadPersistedSnapshot(resultID string) (ScanSnapshot, error) {
	metaPath := resultMetaPath(s.resultsRoot(), resultID)
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return ScanSnapshot{}, err
	}
	var snapshot ScanSnapshot
	if err = json.Unmarshal(data, &snapshot); err != nil {
		return ScanSnapshot{}, err
	}
	return snapshot, nil
}

func (s *Server) loadPersistedAccounts(resultID string) ([]AccountRecord, error) {
	file, err := os.Open(resultAccountsPath(s.resultsRoot(), resultID))
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	reader := bufio.NewScanner(file)
	reader.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	items := make([]AccountRecord, 0, 1024)
	for reader.Scan() {
		line := bytes.TrimSpace(reader.Bytes())
		if len(line) == 0 {
			continue
		}
		var account AccountRecord
		if err = json.Unmarshal(line, &account); err != nil {
			return nil, err
		}
		items = append(items, account)
	}
	if err = reader.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func filterAndSortAccounts(accounts []AccountRecord, search string, status string, sortKey string, onlyFailure bool) []AccountRecord {
	needle := strings.ToLower(strings.TrimSpace(search))
	filtered := make([]AccountRecord, 0, len(accounts))
	for _, account := range accounts {
		if onlyFailure && account.Note == "" && len(account.Windows) > 0 && account.StatusCode < 400 {
			continue
		}
		if status != "" && status != "all" && string(account.Status) != status {
			continue
		}
		if needle != "" {
			if !strings.Contains(strings.ToLower(account.Email), needle) && !strings.Contains(strings.ToLower(account.File), needle) && !strings.Contains(strings.ToLower(account.ID), needle) && !strings.Contains(strings.ToLower(account.Note), needle) {
				continue
			}
		}
		filtered = append(filtered, account)
	}
	sort.Slice(filtered, func(i, j int) bool {
		a, b := filtered[i], filtered[j]
		switch sortKey {
		case "quotaDesc":
			if a.QuotaPercent == b.QuotaPercent {
				return strings.ToLower(a.File) < strings.ToLower(b.File)
			}
			return a.QuotaPercent > b.QuotaPercent
		case "valueDesc":
			if a.USDValue == b.USDValue {
				return strings.ToLower(a.File) < strings.ToLower(b.File)
			}
			return a.USDValue > b.USDValue
		case "emailAsc":
			return a.Email < b.Email
		case "statusDesc":
			left := 0
			right := 0
			if a.Note != "" || a.StatusCode >= 400 {
				left = 1
			}
			if b.Note != "" || b.StatusCode >= 400 {
				right = 1
			}
			if left == right {
				return strings.ToLower(a.File) < strings.ToLower(b.File)
			}
			return left > right
		case "fileAsc":
			fallthrough
		default:
			if a.QuotaPercent == b.QuotaPercent && sortKey == "quotaAsc" {
				return strings.ToLower(a.File) < strings.ToLower(b.File)
			}
			if sortKey == "quotaAsc" {
				return a.QuotaPercent < b.QuotaPercent
			}
			return strings.ToLower(a.File) < strings.ToLower(b.File)
		}
	})
	return filtered
}

func paginateAccounts(items []AccountRecord, page int, pageSize int) (slice []AccountRecord, total int, totalPages int, currentPage int) {
	total = len(items)
	if pageSize <= 0 {
		pageSize = 50
	}
	if total == 0 {
		return []AccountRecord{}, 0, 0, 1
	}
	totalPages = (total + pageSize - 1) / pageSize
	if page <= 0 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}
	return items[start:end], total, totalPages, page
}

func (s *Server) getOrCountImportedDirectoryCount(path string) int {
	if count, ok := s.getCachedImportedDirectoryCount(path); ok {
		return count
	}
	count := countJSONFilesRecursive(path)
	s.setImportedDirectoryCount(path, count)
	return count
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	directories := s.listDirectories()
	staticReady := false
	if strings.TrimSpace(s.config.StaticDir) != "" {
		if _, err := os.Stat(filepath.Join(s.config.StaticDir, "index.html")); err == nil {
			staticReady = true
		}
	}
	s.mu.Lock()
	s.pruneExpiredStateLocked(time.Now())
	cacheEntries := len(s.cache)
	s.mu.Unlock()
	s.writeJSON(w, http.StatusOK, HealthResponse{
		OK:             true,
		AppName:        s.config.AppName,
		Time:           time.Now().Format(time.RFC3339),
		UptimeSeconds:  int64(time.Since(s.start).Seconds()),
		CacheEntries:   cacheEntries,
		DirectoryCount: len(directories),
		StaticReady:    staticReady,
		LogicalCPU:     runtime.NumCPU(),
	})
}

func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w)
		return
	}
	directories := s.listDirectories()
	defaultDir := ""
	defaultRecommended := DetectRecommendedConcurrency(runtime.NumCPU())
	if len(directories) > 0 {
		defaultDir = directories[0].Name
	}
	s.writeJSON(w, http.StatusOK, MetaResponse{
		AppName:          s.config.AppName,
		WorkspaceRoot:    s.config.WorkspaceRoot,
		Directories:      directories,
		DefaultDirectory: defaultDir,
		System: SystemInfo{
			LogicalCPU:             runtime.NumCPU(),
			RecommendedConcurrency: defaultRecommended,
			DetectedMaxConcurrency: DetectRecommendedConcurrency(runtime.NumCPU()),
		},
		DefaultPrice:    s.config.DefaultPrice,
		CacheTTLSeconds: int(s.config.CacheTTL.Seconds()),
	})
}

func (s *Server) handleClearImportedFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w)
		return
	}
	importRoot := filepath.Join(s.config.AppRoot, "imports")
	entries, err := os.ReadDir(importRoot)
	if err != nil && !os.IsNotExist(err) {
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("read imports failed: %v", err))
		return
	}
	removed := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		target := filepath.Join(importRoot, entry.Name())
		if err := os.RemoveAll(target); err != nil {
			s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("remove import directory failed: %v", err))
			return
		}
		removed = append(removed, entry.Name())
	}
	s.mu.Lock()
	s.pruneExpiredStateLocked(time.Now())
	for key, entry := range s.cache {
		if filepath.Clean(entry.snapshot.DirectoryPath) == filepath.Clean(importRoot) || strings.HasPrefix(filepath.Clean(entry.snapshot.DirectoryPath), filepath.Clean(importRoot)+string(os.PathSeparator)) {
			delete(s.cache, key)
		}
	}
	s.invalidateDirectoryListingLocked()
	s.mu.Unlock()
	s.clearImportedDirectoryCountsUnder(importRoot)
	s.writeJSON(w, http.StatusOK, ClearImportedFilesResponse{
		Removed:      removed,
		RemovedCount: len(removed),
	})
}

func (s *Server) handleClearStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w)
		return
	}
	s.mu.Lock()
	clearedCache := len(s.cache)
	clearedJobs := len(s.jobs)
	s.cache = make(map[string]cacheEntry)
	s.jobs = make(map[string]*ScanJob)
	_ = s.clearPersistedResults()
	s.invalidateDirectoryListingLocked()
	remainingCache := len(s.cache)
	remainingJobs := len(s.jobs)
	s.mu.Unlock()
	s.writeJSON(w, http.StatusOK, ClearStatsResponse{
		Cleared:          true,
		ClearedCache:     clearedCache,
		ClearedJobs:      clearedJobs,
		RemainingCache:   remainingCache,
		RemainingRunning: remainingJobs,
	})
}

func (s *Server) handleImportFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w)
		return
	}
	if err := r.ParseMultipartForm(256 << 20); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("parse multipart failed: %v", err))
		return
	}
	folderName := sanitizeFolderName(r.FormValue("folderName"))
	appendMode := strings.EqualFold(strings.TrimSpace(r.FormValue("append")), "true") || strings.TrimSpace(r.FormValue("append")) == "1"
	if folderName == "" {
		folderName = fmt.Sprintf("导入目录_%d", time.Now().Unix())
	}
	paths := r.MultipartForm.Value["paths"]
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		s.writeError(w, http.StatusBadRequest, "未选择任何文件")
		return
	}
	importRoot := filepath.Join(s.config.AppRoot, "imports")
	targetDir := filepath.Join(importRoot, folderName)
	existingCount := 0
	if !appendMode {
		if err := os.RemoveAll(targetDir); err != nil {
			s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("清理旧导入目录失败: %v", err))
			return
		}
		s.removeImportedDirectoryCount(targetDir)
	} else {
		existingCount = s.getOrCountImportedDirectoryCount(targetDir)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("创建导入目录失败: %v", err))
		return
	}
	count := 0
	failed := 0
	for index, fileHeader := range files {
		relPath := fileHeader.Filename
		if index < len(paths) && strings.TrimSpace(paths[index]) != "" {
			relPath = paths[index]
		}
		relPath = normalizeImportedRelativePath(folderName, relPath)
		if relPath == "" || !strings.HasSuffix(strings.ToLower(relPath), ".json") {
			continue
		}
		destination, err := resolveImportDestination(targetDir, relPath)
		if err != nil {
			failed++
			continue
		}
		_, statErr := os.Stat(destination)
		isNewFile := statErr != nil
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			failed++
			continue
		}
		src, err := fileHeader.Open()
		if err != nil {
			failed++
			continue
		}
		data, err := io.ReadAll(src)
		_ = src.Close()
		if err != nil {
			failed++
			continue
		}
		if !json.Valid(bytes.TrimSpace(data)) {
			failed++
			continue
		}
		if err := os.WriteFile(destination, data, 0o644); err != nil {
			failed++
			continue
		}
		if isNewFile {
			count++
		}
	}
	if count == 0 {
		if !appendMode {
			_ = os.RemoveAll(targetDir)
		}
		s.writeError(w, http.StatusBadRequest, "导入失败：未发现有效 JSON 文件")
		return
	}
	if failed > 0 {
		fmt.Printf("import warning: folder=%s imported=%d failed=%d\n", folderName, count, failed)
	}
	jsonCount := existingCount + count
	s.setImportedDirectoryCount(targetDir, jsonCount)
	info := DirectoryInfo{Name: folderName, Path: targetDir, JSONCount: jsonCount, Imported: true}
	s.writeJSON(w, http.StatusOK, ImportFolderResponse{Imported: info})
}

func (s *Server) handleDeleteDirectory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w)
		return
	}
	var req DeleteDirectoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}
	directory := strings.TrimSpace(req.Directory)
	if directory == "" {
		s.writeError(w, http.StatusBadRequest, "directory is required")
		return
	}
	importRoot := filepath.Join(s.config.AppRoot, "imports")
	targetDir := filepath.Join(importRoot, directory)
	cleanRoot := filepath.Clean(importRoot)
	cleanTarget := filepath.Clean(targetDir)
	if !strings.HasPrefix(cleanTarget, cleanRoot) {
		s.writeError(w, http.StatusForbidden, "only imported directories can be deleted")
		return
	}
	if _, err := os.Stat(cleanTarget); err != nil {
		s.writeError(w, http.StatusNotFound, fmt.Sprintf("directory not found: %s", directory))
		return
	}
	if err := os.RemoveAll(cleanTarget); err != nil {
		s.writeError(w, http.StatusInternalServerError, fmt.Sprintf("delete directory failed: %v", err))
		return
	}
	s.invalidateDirectoryCache(cleanTarget)
	s.removeImportedDirectoryCount(cleanTarget)
	s.writeJSON(w, http.StatusOK, DeleteDirectoryResponse{Deleted: directory})
}

func (s *Server) handleScanJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w)
		return
	}
	req, err := s.decodeRequest(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	jobID, err := s.startJob(r.Context(), req, false)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(w, http.StatusAccepted, StartJobResponse{JobID: jobID})
}

func (s *Server) handleRefreshJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w)
		return
	}
	req, err := s.decodeRequest(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Force = true
	jobID, err := s.startJob(r.Context(), req, true)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(w, http.StatusAccepted, StartJobResponse{JobID: jobID})
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w)
		return
	}
	jobID := strings.TrimSpace(r.URL.Query().Get("id"))
	if jobID == "" {
		s.writeError(w, http.StatusBadRequest, "job id is required")
		return
	}
	s.mu.Lock()
	s.pruneExpiredStateLocked(time.Now())
	job, ok := s.jobs[jobID]
	s.mu.Unlock()
	if !ok || job == nil {
		s.writeError(w, http.StatusNotFound, "job not found")
		return
	}
	s.writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleAccountsPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w)
		return
	}
	resultID := strings.TrimSpace(r.URL.Query().Get("resultId"))
	if resultID == "" {
		s.writeError(w, http.StatusBadRequest, "resultId is required")
		return
	}
	page := parseIntOrDefault(r.URL.Query().Get("page"), 1)
	pageSize := parseIntOrDefault(r.URL.Query().Get("pageSize"), 50)
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > maxAccountsPageSize {
		pageSize = maxAccountsPageSize
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	sortKey := strings.TrimSpace(r.URL.Query().Get("sort"))
	if sortKey == "" {
		sortKey = "quotaAsc"
	}
	onlyFailure := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("onlyFailure")), "true")
	if _, err := s.loadPersistedSnapshot(resultID); err != nil {
		s.writeError(w, http.StatusNotFound, fmt.Sprintf("load persisted snapshot failed: %v", err))
		return
	}
	accounts, ok := s.getCachedAccountsByResultID(resultID)
	if !ok {
		var err error
		accounts, err = s.loadPersistedAccounts(resultID)
		if err != nil {
			s.writeError(w, http.StatusNotFound, fmt.Sprintf("load persisted accounts failed: %v", err))
			return
		}
	}
	filtered := filterAndSortAccounts(accounts, search, status, sortKey, onlyFailure)
	items, total, totalPages, currentPage := paginateAccounts(filtered, page, pageSize)
	s.writeJSON(w, http.StatusOK, AccountsPageResponse{
		ResultID:    resultID,
		Page:        currentPage,
		PageSize:    pageSize,
		Total:       total,
		TotalPages:  totalPages,
		Search:      search,
		Status:      status,
		Sort:        sortKey,
		OnlyFailure: onlyFailure,
		Items:       items,
	})
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w)
		return
	}
	req, err := s.decodeRequest(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	snapshot, err := s.loadSnapshot(r.Context(), req, false)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) startJob(ctx context.Context, req ScanRequest, mergeSelected bool) (string, error) {
	dirInfo, err := s.resolveDirectory(req.Directory)
	if err != nil {
		return "", err
	}
	total := dirInfo.JSONCount
	if len(req.AccountIDs) > 0 {
		total = len(req.AccountIDs)
	}
	if total < 0 {
		total = 0
	}
	jobID := fmt.Sprintf("job-%d", time.Now().UnixNano())
	job := &ScanJob{
		ID:        jobID,
		Status:    "running",
		Directory: dirInfo.Name,
		Total:     total,
		StartedAt: time.Now().Format(time.RFC3339),
	}
	s.mu.Lock()
	s.pruneExpiredStateLocked(time.Now())
	s.jobs[jobID] = job
	s.mu.Unlock()

	go func() {
		snapshot, err := s.loadSnapshotWithProgress(context.Background(), req, mergeSelected, func(done, total int) {
			s.updateJobProgress(jobID, done, total)
		})
		if err != nil {
			s.finishJob(jobID, "failed", err.Error(), nil)
			return
		}
		s.finishJob(jobID, "completed", "", &snapshot)
	}()
	return jobID, nil
}

func (s *Server) updateJobProgress(jobID string, done, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[jobID]
	if job == nil {
		return
	}
	job.Done = done
	job.Total = total
	if total > 0 {
		job.Percent = round2(float64(done) * 100 / float64(total))
	}
	job.Message = fmt.Sprintf("已处理 %d / %d", done, total)
}

func (s *Server) finishJob(jobID, status, message string, snapshot *ScanSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[jobID]
	if job == nil {
		return
	}
	job.Status = status
	job.Message = message
	job.FinishedAt = time.Now().Format(time.RFC3339)
	job.finishedAtTime = time.Now()
	if snapshot != nil {
		persistedSnapshot, err := s.persistSnapshot(*snapshot)
		if err != nil {
			job.Status = "failed"
			job.Message = fmt.Sprintf("persist snapshot failed: %v", err)
			job.Snapshot = nil
			return
		}
		s.attachResultIDToCachedSnapshotLocked(persistedSnapshot.ResultID, *snapshot)
		job.Snapshot = &persistedSnapshot
		job.Done = persistedSnapshot.Summary.TotalAccounts
		job.Total = persistedSnapshot.Summary.TotalAccounts
		job.Percent = 100
		return
	}
	job.Snapshot = nil
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeMethodNotAllowed(w)
		return
	}
	req, err := s.decodeRequest(r)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Force = true
	snapshot, err := s.loadSnapshot(r.Context(), req, true)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeMethodNotAllowed(w)
		return
	}
	query := r.URL.Query()
	req := ScanRequest{
		Directory:       strings.TrimSpace(query.Get("directory")),
		FullValueUSD:    parseFloatOrDefault(query.Get("fullValueUSD"), s.config.DefaultPrice),
		AutoConcurrency: query.Get("autoConcurrency") != "false",
		Concurrency:     parseIntOrDefault(query.Get("concurrency"), 0),
		Force:           query.Get("force") == "true",
	}
	snapshot, err := s.loadSnapshot(r.Context(), req, false)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", csvFileName(snapshot.Directory)))
	writer := csv.NewWriter(w)
	defer writer.Flush()
	_ = writer.Write([]string{"id", "file", "email", "plan", "quotaPercent", "usdValue", "resetDate", "status", "statusCode", "lastRefresh", "expiredAt", "note"})
	for _, account := range snapshot.Accounts {
		_ = writer.Write([]string{
			account.ID,
			account.File,
			account.Email,
			account.Plan,
			fmt.Sprintf("%.2f", account.QuotaPercent),
			fmt.Sprintf("%.2f", account.USDValue),
			account.ResetDate,
			string(account.Status),
			fmt.Sprintf("%d", account.StatusCode),
			account.LastRefresh,
			account.ExpiredAt,
			account.Note,
		})
	}
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	staticDir := strings.TrimSpace(s.config.StaticDir)
	if staticDir == "" {
		s.writeStaticFallback(w)
		return
	}
	filePath := filepath.Join(staticDir, filepath.Clean(strings.TrimPrefix(r.URL.Path, "/")))
	if r.URL.Path == "/" {
		filePath = filepath.Join(staticDir, "index.html")
	}
	if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
		http.ServeFile(w, r, filePath)
		return
	}
	indexPath := filepath.Join(staticDir, "index.html")
	if _, err := os.Stat(indexPath); err == nil {
		http.ServeFile(w, r, indexPath)
		return
	}
	s.writeStaticFallback(w)
}

func (s *Server) writeStaticFallback(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8"><title>Codex普号额度概览</title></head><body style="font-family:sans-serif;padding:32px"><h1>Codex普号额度概览</h1><p>前端构建产物不存在，请先在 <code>web</code> 目录执行 <code>npm install</code> 与 <code>npm run build</code>，再重新启动服务。</p></body></html>`))
}

func (s *Server) decodeRequest(r *http.Request) (ScanRequest, error) {
	var req ScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return ScanRequest{}, fmt.Errorf("invalid request body: %w", err)
	}
	if req.FullValueUSD <= 0 {
		req.FullValueUSD = s.config.DefaultPrice
	}
	if !req.AutoConcurrency && req.Concurrency <= 0 {
		req.AutoConcurrency = true
	}
	return req, nil
}

func (s *Server) loadSnapshot(ctx context.Context, req ScanRequest, mergeSelected bool) (ScanSnapshot, error) {
	return s.loadSnapshotWithProgress(ctx, req, mergeSelected, nil)
}

func (s *Server) loadSnapshotWithProgress(ctx context.Context, req ScanRequest, mergeSelected bool, progressFn func(done, total int)) (ScanSnapshot, error) {
	dirInfo, err := s.resolveDirectory(req.Directory)
	if err != nil {
		return ScanSnapshot{}, err
	}
	cacheKey := dirInfo.Path + "|" + fmt.Sprintf("%.2f", req.FullValueUSD)
	if !req.Force && len(req.AccountIDs) == 0 {
		if snapshot, ok := s.getCachedSnapshot(cacheKey); ok {
			return snapshot, nil
		}
	}
	selected := make(map[string]struct{}, len(req.AccountIDs))
	for _, id := range req.AccountIDs {
		trimmed := normalizeSelectedID(id)
		if trimmed != "" {
			selected[trimmed] = struct{}{}
		}
	}
	if mergeSelected && len(selected) > 0 {
		existing, ok := s.loadMergeBaseSnapshot(cacheKey, req.ResultID)
		if !ok {
			return ScanSnapshot{}, fmt.Errorf("缺少可用于合并的基础扫描结果，请先完成一次完整扫描")
		}
		selectedPaths := make([]string, 0, len(req.AccountIDs))
		seenSelected := make(map[string]struct{}, len(selected))
		for _, id := range req.AccountIDs {
			raw := strings.TrimSpace(strings.ReplaceAll(id, "\\", "/"))
			raw = strings.TrimPrefix(raw, "./")
			raw = strings.TrimPrefix(raw, "/")
			raw = pathCleanSlash(raw)
			normalized := normalizeSelectedID(raw)
			if normalized == "" {
				continue
			}
			if _, duplicated := seenSelected[normalized]; duplicated {
				continue
			}
			seenSelected[normalized] = struct{}{}
			selectedPaths = append(selectedPaths, raw)
		}
		if len(selectedPaths) == 0 {
			for _, account := range existing.Accounts {
				candidates := []string{account.ID, account.File}
				for _, candidate := range candidates {
					normalized := normalizeSelectedID(candidate)
					if normalized == "" {
						continue
					}
					if _, wanted := selected[normalized]; !wanted {
						continue
					}
					if _, duplicated := seenSelected[normalized]; duplicated {
						continue
					}
					seenSelected[normalized] = struct{}{}
					selectedPaths = append(selectedPaths, candidate)
					break
				}
			}
		}
		if len(selectedPaths) == 0 {
			return ScanSnapshot{}, fmt.Errorf("所选账号未在当前目录中匹配到可刷新文件")
		}
		snapshot, err := ScanSelectedFiles(ctx, scanOptions{
			DirectoryPath:    dirInfo.Path,
			DirectoryName:    dirInfo.Name,
			FullValueUSD:     req.FullValueUSD,
			AutoConcurrency:  req.AutoConcurrency || req.Concurrency <= 0,
			RequestedWorkers: req.Concurrency,
			Progress:         progressFn,
		}, selectedPaths)
		if err != nil {
			return ScanSnapshot{}, err
		}
		if len(snapshot.Accounts) == 0 {
			return ScanSnapshot{}, fmt.Errorf("所选账号未在当前目录中匹配到可刷新文件")
		}
		snapshot = mergeSnapshots(existing, snapshot)
		s.setCachedSnapshot(cacheKey, snapshot, req.FullValueUSD)
		return snapshot, nil
	}
	snapshot, err := ScanDirectory(ctx, scanOptions{
		DirectoryPath:    dirInfo.Path,
		DirectoryName:    dirInfo.Name,
		FullValueUSD:     req.FullValueUSD,
		AutoConcurrency:  req.AutoConcurrency || req.Concurrency <= 0,
		RequestedWorkers: req.Concurrency,
		SelectedIDs:      selected,
		Progress:         progressFn,
	})
	if err != nil {
		return ScanSnapshot{}, err
	}
	s.setCachedSnapshot(cacheKey, snapshot, req.FullValueUSD)
	return snapshot, nil
}

func (s *Server) invalidateDirectoryCache(directoryPath string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, entry := range s.cache {
		if entry.snapshot.DirectoryPath == directoryPath {
			delete(s.cache, key)
		}
	}
	s.invalidateDirectoryListingLocked()
}

func mergeSnapshots(existing, partial ScanSnapshot) ScanSnapshot {
	if len(partial.Accounts) == 0 {
		return existing
	}
	index := make(map[string]int, len(existing.Accounts))
	for i := range existing.Accounts {
		index[existing.Accounts[i].ID] = i
	}
	for _, account := range partial.Accounts {
		if pos, ok := index[account.ID]; ok {
			existing.Accounts[pos] = account
		} else {
			existing.Accounts = append(existing.Accounts, account)
		}
	}
	sort.Slice(existing.Accounts, func(i, j int) bool {
		return strings.ToLower(existing.Accounts[i].File) < strings.ToLower(existing.Accounts[j].File)
	})
	existing.ScannedAt = partial.ScannedAt
	existing.DurationMs = partial.DurationMs
	existing.ConcurrencyUsed = partial.ConcurrencyUsed
	existing.RecommendedConcurrency = partial.RecommendedConcurrency
	existing.LogicalCPU = partial.LogicalCPU
	existing.Summary = buildSummary(existing.Accounts)
	return existing
}

func (s *Server) getCachedSnapshot(key string) (ScanSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredStateLocked(time.Now())
	entry, ok := s.cache[key]
	if !ok {
		return ScanSnapshot{}, false
	}
	return entry.snapshot, true
}

func (s *Server) setCachedSnapshot(key string, snapshot ScanSnapshot, fullValue float64) {
	s.mu.Lock()
	s.pruneExpiredStateLocked(time.Now())
	s.cache[key] = cacheEntry{fullValue: fullValue, snapshot: snapshot, createdAt: time.Now()}
	s.mu.Unlock()
}

func (s *Server) resolveDirectory(requested string) (DirectoryInfo, error) {
	directories := s.listDirectories()
	if strings.TrimSpace(requested) == "" {
		if len(directories) == 0 {
			return DirectoryInfo{}, fmt.Errorf("workspace root 下未发现可扫描的认证目录，请手动输入目录路径")
		}
		return directories[0], nil
	}
	requested = strings.TrimSpace(requested)
	for _, directory := range directories {
		if strings.EqualFold(directory.Name, requested) || strings.EqualFold(directory.Path, requested) {
			return directory, nil
		}
	}
	if info, err := os.Stat(requested); err == nil && info.IsDir() {
		count := countJSONFilesRecursive(requested)
		if count == 0 {
			return DirectoryInfo{}, fmt.Errorf("directory has no json files: %s", requested)
		}
		return DirectoryInfo{Name: filepath.Base(requested), Path: requested, JSONCount: count}, nil
	}
	candidate := filepath.Join(s.config.WorkspaceRoot, requested)
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		count := countJSONFilesRecursive(candidate)
		if count == 0 {
			return DirectoryInfo{}, fmt.Errorf("directory has no json files: %s", candidate)
		}
		return DirectoryInfo{Name: filepath.Base(candidate), Path: candidate, JSONCount: count}, nil
	}
	return DirectoryInfo{}, fmt.Errorf("directory not found: %s", requested)
}

func (s *Server) listDirectories() []DirectoryInfo {
	now := time.Now()
	s.mu.Lock()
	s.pruneExpiredStateLocked(now)
	if len(s.directoryListingCache) > 0 && now.Sub(s.directoryListingCacheAt) <= directoryListingCacheTTL {
		cached := cloneDirectoryInfos(s.directoryListingCache)
		s.mu.Unlock()
		return cached
	}
	s.mu.Unlock()

	directories := make([]DirectoryInfo, 0)
	directories = append(directories, s.collectWorkspaceDirectories()...)
	directories = append(directories, s.collectImportedDirectories()...)
	sort.Slice(directories, func(i, j int) bool {
		if directories[i].JSONCount == directories[j].JSONCount {
			return strings.ToLower(directories[i].Name) < strings.ToLower(directories[j].Name)
		}
		return directories[i].JSONCount > directories[j].JSONCount
	})
	s.mu.Lock()
	s.directoryListingCache = cloneDirectoryInfos(directories)
	s.directoryListingCacheAt = now
	s.mu.Unlock()
	return directories
}

func (s *Server) collectWorkspaceDirectories() []DirectoryInfo {
	entries, err := os.ReadDir(s.config.WorkspaceRoot)
	if err != nil {
		return nil
	}
	ignored := map[string]struct{}{
		"Codex普号额度概览":  {},
		"codex-额度查询前端": {},
		"CLIProxyAPI":  {},
	}
	directories := make([]DirectoryInfo, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if _, skip := ignored[name]; skip {
			continue
		}
		fullPath := filepath.Join(s.config.WorkspaceRoot, name)
		count := countJSONFiles(fullPath)
		if count == 0 {
			continue
		}
		directories = append(directories, DirectoryInfo{Name: name, Path: fullPath, JSONCount: count, Imported: false})
	}
	return directories
}

func (s *Server) collectImportedDirectories() []DirectoryInfo {
	importRoot := filepath.Join(s.config.AppRoot, "imports")
	entries, err := os.ReadDir(importRoot)
	if err != nil {
		return nil
	}
	directories := make([]DirectoryInfo, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		fullPath := filepath.Join(importRoot, entry.Name())
		count := s.getOrCountImportedDirectoryCount(fullPath)
		if count == 0 {
			continue
		}
		directories = append(directories, DirectoryInfo{Name: entry.Name(), Path: fullPath, JSONCount: count, Imported: true})
	}
	return directories
}

func countJSONFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			count++
		}
	}
	return count
}

func countJSONFilesRecursive(dir string) int {
	count := 0
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(d.Name()), ".json") {
			count++
		}
		return nil
	})
	return count
}

func sanitizeFolderName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	name = replacer.Replace(name)
	if len(name) > 80 {
		name = name[:80]
	}
	return strings.TrimSpace(name)
}

func sanitizeRelativePath(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "/")
	if strings.Contains(path, "..") {
		return ""
	}
	return path
}

func normalizeImportedRelativePath(folderName, path string) string {
	path = sanitizeRelativePath(path)
	if path == "" {
		return ""
	}
	segments := strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' })
	if len(segments) == 0 {
		return ""
	}
	if len(segments) > 1 && strings.EqualFold(sanitizeFolderName(segments[0]), folderName) {
		segments = segments[1:]
	}
	if len(segments) == 0 {
		return ""
	}
	return strings.Join(segments, "/")
}

func resolveImportDestination(root, relPath string) (string, error) {
	relPath = sanitizeRelativePath(relPath)
	if relPath == "" {
		return "", fmt.Errorf("relative path is empty")
	}
	destination := filepath.Clean(filepath.Join(root, filepath.FromSlash(relPath)))
	cleanRoot := filepath.Clean(root)
	if destination != cleanRoot && !strings.HasPrefix(destination, cleanRoot+string(os.PathSeparator)) {
		return "", fmt.Errorf("destination escaped import root")
	}
	if len(destination) <= 240 {
		return destination, nil
	}
	ext := filepath.Ext(destination)
	base := strings.TrimSuffix(filepath.Base(destination), ext)
	if base == "" {
		base = "imported"
	}
	hash := sha1.Sum([]byte(relPath))
	hashText := hex.EncodeToString(hash[:6])
	if len(base) > 64 {
		base = base[:64]
	}
	shortName := fmt.Sprintf("%s_%s%s", base, hashText, ext)
	destination = filepath.Join(filepath.Dir(destination), shortName)
	if destination != cleanRoot && !strings.HasPrefix(destination, cleanRoot+string(os.PathSeparator)) {
		return "", fmt.Errorf("shortened destination escaped import root")
	}
	return destination, nil
}

func flattenImportedFileName(path string) string {
	flattened := strings.ReplaceAll(path, "/", "__")
	if flattened == "" {
		return fmt.Sprintf("imported_%d.json", time.Now().UnixNano())
	}
	return flattened
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type gzipResponseWriter struct {
	http.ResponseWriter
	writer *gzip.Writer
}

func (g *gzipResponseWriter) WriteHeader(statusCode int) {
	g.Header().Del("Content-Length")
	g.ResponseWriter.WriteHeader(statusCode)
}

func (g *gzipResponseWriter) Write(data []byte) (int, error) {
	return g.writer.Write(data)
}

func (g *gzipResponseWriter) Flush() {
	_ = g.writer.Flush()
	if flusher, ok := g.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (s *Server) gzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") || r.Method == http.MethodHead || !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
		gz := gzip.NewWriter(w)
		defer func() { _ = gz.Close() }()
		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, writer: gz}, r)
	})
}

func (s *Server) writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) writeError(w http.ResponseWriter, statusCode int, message string) {
	s.writeJSON(w, statusCode, map[string]any{"error": message})
}

func (s *Server) writeMethodNotAllowed(w http.ResponseWriter) {
	s.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func parseFloatOrDefault(value string, fallback float64) float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseIntOrDefault(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func csvFileName(directory string) string {
	name := strings.TrimSpace(directory)
	if name == "" {
		name = "accounts"
	}
	return name + "-quota-report.csv"
}

func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
