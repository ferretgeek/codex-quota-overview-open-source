package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistSnapshotAndAccountsPage(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	server := NewServer(ServerConfig{
		AppRoot:       root,
		WorkspaceRoot: root,
		StaticDir:     "",
		CacheTTL:      10 * time.Second,
		AppName:       "test",
		DefaultPrice:  7.5,
	})

	snapshot := ScanSnapshot{
		Directory:              "demo",
		DirectoryPath:          filepath.Join(root, "imports", "demo"),
		ScannedAt:              time.Now().Format(time.RFC3339),
		DurationMs:             1234,
		FullValueUSD:           7.5,
		AutoConcurrency:        true,
		ConcurrencyUsed:        40,
		RecommendedConcurrency: 40,
		LogicalCPU:             2,
		Summary: Summary{
			TotalAccounts: 3,
			SuccessCount:  3,
			FailedCount:   0,
			TotalValueUSD: 14.25,
		},
		Accounts: []AccountRecord{
			{ID: "a", File: "a.json", Email: "a@example.com", QuotaPercent: 90, USDValue: 6.75, Status: AccountStatusNormal},
			{ID: "b", File: "b.json", Email: "b@example.com", QuotaPercent: 10, USDValue: 0.75, Status: AccountStatusDepleted, Note: "low"},
			{ID: "c", File: "c.json", Email: "c@example.com", QuotaPercent: 90, USDValue: 6.75, Status: AccountStatusNormal},
		},
	}

	clientSnapshot, err := server.persistSnapshot(snapshot)
	if err != nil {
		t.Fatalf("persistSnapshot failed: %v", err)
	}
	if clientSnapshot.ResultID == "" {
		t.Fatalf("expected result id")
	}
	if !clientSnapshot.AccountsPartial {
		t.Fatalf("expected partial accounts flag")
	}
	if clientSnapshot.StoredAccountCount != 3 {
		t.Fatalf("expected stored count 3, got %d", clientSnapshot.StoredAccountCount)
	}
	if len(clientSnapshot.Accounts) != 0 {
		t.Fatalf("expected client snapshot accounts to be empty")
	}
	if len(clientSnapshot.PreviewAccounts) == 0 || clientSnapshot.PreviewAccounts[0].ID != "b" {
		t.Fatalf("expected preview accounts to include lowest quota account first")
	}

	loadedAccounts, err := server.loadPersistedAccounts(clientSnapshot.ResultID)
	if err != nil {
		t.Fatalf("loadPersistedAccounts failed: %v", err)
	}
	if len(loadedAccounts) != 3 {
		t.Fatalf("expected 3 loaded accounts, got %d", len(loadedAccounts))
	}

	req := httptest.NewRequest(http.MethodGet, "/api/accounts?resultId="+clientSnapshot.ResultID+"&page=1&pageSize=2&sort=quotaAsc", nil)
	resp := httptest.NewRecorder()
	server.handleAccountsPage(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", resp.Code, resp.Body.String())
	}
	var page AccountsPageResponse
	if err = json.Unmarshal(resp.Body.Bytes(), &page); err != nil {
		t.Fatalf("unmarshal page failed: %v", err)
	}
	if page.Total != 3 || page.PageSize != 2 || len(page.Items) != 2 {
		t.Fatalf("unexpected paging result: %+v", page)
	}
	if page.Items[0].ID != "b" {
		t.Fatalf("expected lowest quota record first, got %s", page.Items[0].ID)
	}
	if page.TotalPages != 2 {
		t.Fatalf("expected total pages 2, got %d", page.TotalPages)
	}
}

func TestLoadMergeBaseSnapshotFallsBackToPersistedResult(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	server := NewServer(ServerConfig{
		AppRoot:       root,
		WorkspaceRoot: root,
		CacheTTL:      10 * time.Second,
		AppName:       "test",
		DefaultPrice:  7.5,
	})

	snapshot := ScanSnapshot{
		Directory:     "demo",
		DirectoryPath: filepath.Join(root, "imports", "demo"),
		ScannedAt:     time.Now().Format(time.RFC3339),
		Summary:       Summary{TotalAccounts: 2, SuccessCount: 2, TotalValueUSD: 10},
		Accounts: []AccountRecord{
			{ID: "folder/a.json", File: "folder/a.json", Email: "a@example.com", QuotaPercent: 60, USDValue: 4.5, Status: AccountStatusNormal},
			{ID: "folder/b.json", File: "folder/b.json", Email: "b@example.com", QuotaPercent: 80, USDValue: 6.0, Status: AccountStatusNormal},
		},
	}
	persisted, err := server.persistSnapshot(snapshot)
	if err != nil {
		t.Fatalf("persistSnapshot failed: %v", err)
	}
	merged, ok := server.loadMergeBaseSnapshot("missing-cache", persisted.ResultID)
	if !ok {
		t.Fatalf("expected persisted snapshot fallback")
	}
	if len(merged.Accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(merged.Accounts))
	}
	if merged.AccountsPartial {
		t.Fatalf("expected full merged snapshot")
	}
}

func TestCollectScanFilesMatchesNormalizedSelectedIDs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	nested := filepath.Join(root, "sample-pool", "group-a")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	filePath := filepath.Join(nested, "sample-user@example.test.json")
	if err := os.WriteFile(filePath, []byte(`{"type":"codex"}`), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	selected := map[string]struct{}{
		normalizeSelectedID(`.\sample-pool\group-a\SAMPLE-USER@EXAMPLE.TEST.JSON`): {},
	}
	files, err := collectScanFiles(root, selected)
	if err != nil {
		t.Fatalf("collectScanFiles failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 matched file, got %d", len(files))
	}
}

func TestLoadSnapshotWithProgressMergesSelectedPathsDirectly(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	directoryName := "demo"
	directoryPath := filepath.Join(root, "imports", directoryName)
	relPath := filepath.ToSlash(filepath.Join("pool", "subdir", "account.json"))
	filePath := filepath.Join(directoryPath, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(filePath, []byte(`{"type":"other","email":"demo@example.com"}`), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}

	server := NewServer(ServerConfig{
		AppRoot:       root,
		WorkspaceRoot: root,
		CacheTTL:      10 * time.Second,
		AppName:       "test",
		DefaultPrice:  7.5,
	})

	baseSnapshot := ScanSnapshot{
		Directory:     directoryName,
		DirectoryPath: directoryPath,
		ScannedAt:     time.Now().Format(time.RFC3339),
		FullValueUSD:  7.5,
		Summary: Summary{
			TotalAccounts: 1,
			SuccessCount:  1,
		},
		Accounts: []AccountRecord{{
			ID:           relPath,
			File:         relPath,
			Email:        "demo@example.com",
			QuotaPercent: 88,
			USDValue:     6.6,
			Status:       AccountStatusNormal,
		}},
	}
	persisted, err := server.persistSnapshot(baseSnapshot)
	if err != nil {
		t.Fatalf("persistSnapshot failed: %v", err)
	}

	merged, err := server.loadSnapshotWithProgress(context.Background(), ScanRequest{
		Directory:       directoryName,
		ResultID:        persisted.ResultID,
		FullValueUSD:    7.5,
		AutoConcurrency: false,
		Concurrency:     1,
		Force:           true,
		AccountIDs:      []string{relPath},
	}, true, nil)
	if err != nil {
		t.Fatalf("loadSnapshotWithProgress failed: %v", err)
	}
	if len(merged.Accounts) != 1 {
		t.Fatalf("expected 1 merged account, got %d", len(merged.Accounts))
	}
	if merged.Accounts[0].ID != relPath {
		t.Fatalf("expected merged id %q, got %q", relPath, merged.Accounts[0].ID)
	}
	if merged.Accounts[0].Status != AccountStatusDisabled {
		t.Fatalf("expected refreshed status disabled for unsupported auth, got %q", merged.Accounts[0].Status)
	}
}
