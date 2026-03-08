package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectRecommendedConcurrencyUsesPerCPUThreadRule(t *testing.T) {
	t.Parallel()

	tests := []struct {
		logicalCPU int
		want       int
	}{
		{logicalCPU: 0, want: 20},
		{logicalCPU: 1, want: 20},
		{logicalCPU: 4, want: 80},
		{logicalCPU: 16, want: 320},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("cpu_%d", tc.logicalCPU), func(t *testing.T) {
			t.Parallel()
			if got := DetectRecommendedConcurrency(tc.logicalCPU); got != tc.want {
				t.Fatalf("DetectRecommendedConcurrency(%d) = %d, want %d", tc.logicalCPU, got, tc.want)
			}
		})
	}
}

func TestScanDirectoryUsesRelativePathForNestedDuplicateFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeTestAuthFile(t, filepath.Join(root, "group-a", "same.json"), authFile{Type: "codex", Email: "a@example.com"})
	writeTestAuthFile(t, filepath.Join(root, "group-b", "same.json"), authFile{Type: "codex", Email: "b@example.com"})

	snapshot, err := ScanDirectory(context.Background(), scanOptions{
		DirectoryPath:    root,
		DirectoryName:    "nested",
		FullValueUSD:     7.5,
		AutoConcurrency:  false,
		RequestedWorkers: 2,
	})
	if err != nil {
		t.Fatalf("ScanDirectory failed: %v", err)
	}
	if snapshot.Summary.TotalAccounts != 2 {
		t.Fatalf("total accounts = %d, want 2", snapshot.Summary.TotalAccounts)
	}
	got := map[string]string{}
	for _, account := range snapshot.Accounts {
		got[account.ID] = account.Email
	}
	if got["group-a/same.json"] != "a@example.com" {
		t.Fatalf("missing group-a/same.json record: %#v", got)
	}
	if got["group-b/same.json"] != "b@example.com" {
		t.Fatalf("missing group-b/same.json record: %#v", got)
	}
}

func TestScanDirectoryLargeSyntheticDataset(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	const totalFiles = 10000
	for index := 0; index < totalFiles; index++ {
		dir := filepath.Join(root, fmt.Sprintf("bucket-%03d", index%100))
		writeTestAuthFile(t, filepath.Join(dir, fmt.Sprintf("account-%05d.json", index)), authFile{Type: "codex", Email: fmt.Sprintf("user-%05d@example.com", index)})
	}

	snapshot, err := ScanDirectory(context.Background(), scanOptions{
		DirectoryPath:    root,
		DirectoryName:    "large",
		FullValueUSD:     7.5,
		AutoConcurrency:  false,
		RequestedWorkers: 64,
	})
	if err != nil {
		t.Fatalf("ScanDirectory failed: %v", err)
	}
	if snapshot.Summary.TotalAccounts != totalFiles {
		t.Fatalf("total accounts = %d, want %d", snapshot.Summary.TotalAccounts, totalFiles)
	}
	if snapshot.Summary.FailedCount != totalFiles {
		t.Fatalf("failed count = %d, want %d because synthetic files do not include tokens", snapshot.Summary.FailedCount, totalFiles)
	}
	if snapshot.ConcurrencyUsed != 64 {
		t.Fatalf("concurrency used = %d, want 64", snapshot.ConcurrencyUsed)
	}
}

func BenchmarkScanDirectorySynthetic50000(b *testing.B) {
	benchmarkSyntheticScan(b, 50000, 128)
}

func BenchmarkScanDirectorySynthetic100000(b *testing.B) {
	benchmarkSyntheticScan(b, 100000, 160)
}

func benchmarkSyntheticScan(b *testing.B, totalFiles int, workers int) {
	root := b.TempDir()
	for index := 0; index < totalFiles; index++ {
		dir := filepath.Join(root, fmt.Sprintf("bucket-%03d", index%500))
		writeTestAuthFileFromBytes(b, filepath.Join(dir, fmt.Sprintf("account-%05d.json", index)), []byte(fmt.Sprintf(`{"type":"codex","email":"user-%05d@example.com"}`, index)))
	}

	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		snapshot, err := ScanDirectory(context.Background(), scanOptions{
			DirectoryPath:    root,
			DirectoryName:    "benchmark",
			FullValueUSD:     7.5,
			AutoConcurrency:  false,
			RequestedWorkers: workers,
		})
		if err != nil {
			b.Fatalf("ScanDirectory failed: %v", err)
		}
		if snapshot.Summary.TotalAccounts != totalFiles {
			b.Fatalf("total accounts = %d, want %d", snapshot.Summary.TotalAccounts, totalFiles)
		}
	}
}

func writeTestAuthFile(t *testing.T, path string, payload authFile) {
	t.Helper()
	writeTestAuthFileFromBytes(t, path, []byte(fmt.Sprintf(`{"type":%q,"email":%q}`, payload.Type, payload.Email)))
}

func writeTestAuthFileFromBytes(tb testing.TB, path string, data []byte) {
	tb.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		tb.Fatalf("WriteFile failed: %v", err)
	}
}
