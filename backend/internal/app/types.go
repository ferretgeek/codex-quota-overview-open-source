package app

import "time"

type AccountStatus string

const (
	AccountStatusNormal   AccountStatus = "normal"
	AccountStatusDepleted AccountStatus = "depleted"
	AccountStatusExpired  AccountStatus = "expired"
	AccountStatusDisabled AccountStatus = "disabled"
)

type ServerConfig struct {
	AppRoot       string
	WorkspaceRoot string
	StaticDir     string
	CacheTTL      time.Duration
	AppName       string
	DefaultPrice  float64
}

type ImportFolderResponse struct {
	Imported DirectoryInfo `json:"imported"`
}

type DeleteDirectoryRequest struct {
	Directory string `json:"directory"`
}

type DeleteDirectoryResponse struct {
	Deleted string `json:"deleted"`
}

type ClearImportedFilesResponse struct {
	Removed      []string `json:"removed"`
	RemovedCount int      `json:"removedCount"`
}

type ClearStatsResponse struct {
	Cleared          bool `json:"cleared"`
	ClearedCache     int  `json:"clearedCache"`
	ClearedJobs      int  `json:"clearedJobs"`
	RemainingCache   int  `json:"remainingCache"`
	RemainingRunning int  `json:"remainingRunning"`
}

type DirectoryInfo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	JSONCount int    `json:"jsonCount"`
	Imported  bool   `json:"imported"`
}

type SystemInfo struct {
	LogicalCPU             int `json:"logicalCPU"`
	RecommendedConcurrency int `json:"recommendedConcurrency"`
	DetectedMaxConcurrency int `json:"detectedMaxConcurrency"`
}

type HealthResponse struct {
	OK             bool   `json:"ok"`
	AppName        string `json:"appName"`
	Time           string `json:"time"`
	UptimeSeconds  int64  `json:"uptimeSeconds"`
	CacheEntries   int    `json:"cacheEntries"`
	DirectoryCount int    `json:"directoryCount"`
	StaticReady    bool   `json:"staticReady"`
	LogicalCPU     int    `json:"logicalCPU"`
}

type MetaResponse struct {
	AppName          string          `json:"appName"`
	WorkspaceRoot    string          `json:"workspaceRoot"`
	Directories      []DirectoryInfo `json:"directories"`
	DefaultDirectory string          `json:"defaultDirectory"`
	System           SystemInfo      `json:"system"`
	DefaultPrice     float64         `json:"defaultPrice"`
	CacheTTLSeconds  int             `json:"cacheTTLSeconds"`
}

type ScanRequest struct {
	Directory       string   `json:"directory"`
	ResultID        string   `json:"resultId,omitempty"`
	FullValueUSD    float64  `json:"fullValueUSD"`
	AutoConcurrency bool     `json:"autoConcurrency"`
	Concurrency     int      `json:"concurrency"`
	Force           bool     `json:"force"`
	AccountIDs      []string `json:"accountIds"`
}

type CodexUsageWindow struct {
	Label              string   `json:"label"`
	UsedPercent        *float64 `json:"usedPercent,omitempty"`
	RemainingPercent   *float64 `json:"remainingPercent,omitempty"`
	LimitWindowSeconds *int64   `json:"limitWindowSeconds,omitempty"`
	ResetAfterSeconds  *int64   `json:"resetAfterSeconds,omitempty"`
	ResetAt            *int64   `json:"resetAt,omitempty"`
	ResetAtISO         string   `json:"resetAtISO,omitempty"`
}

type AccountRecord struct {
	ID           string             `json:"id"`
	File         string             `json:"file"`
	Email        string             `json:"email"`
	Plan         string             `json:"plan"`
	QuotaPercent float64            `json:"quotaPercent"`
	USDValue     float64            `json:"usdValue"`
	ResetDate    string             `json:"resetDate,omitempty"`
	Status       AccountStatus      `json:"status"`
	Disabled     bool               `json:"disabled"`
	LastRefresh  string             `json:"lastRefresh,omitempty"`
	ExpiredAt    string             `json:"expiredAt,omitempty"`
	StatusCode   int                `json:"statusCode,omitempty"`
	Note         string             `json:"note,omitempty"`
	Windows      []CodexUsageWindow `json:"windows,omitempty"`
}

type QuotaDistribution struct {
	Healthy  int `json:"healthy"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Depleted int `json:"depleted"`
}

type Summary struct {
	TotalAccounts        int               `json:"totalAccounts"`
	SuccessCount         int               `json:"successCount"`
	FailedCount          int               `json:"failedCount"`
	MonthlyGrowthPercent float64           `json:"monthlyGrowthPercent"`
	TotalValueUSD        float64           `json:"totalValueUSD"`
	AverageQuotaPercent  float64           `json:"averageQuotaPercent"`
	MinQuotaPercent      float64           `json:"minQuotaPercent"`
	MaxQuotaPercent      float64           `json:"maxQuotaPercent"`
	QuotaDistribution    QuotaDistribution `json:"quotaDistribution"`
}

type ScanSnapshot struct {
	ResultID               string          `json:"resultId,omitempty"`
	Directory              string          `json:"directory"`
	DirectoryPath          string          `json:"directoryPath"`
	ScannedAt              string          `json:"scannedAt"`
	DurationMs             int64           `json:"durationMs"`
	FullValueUSD           float64         `json:"fullValueUSD"`
	AutoConcurrency        bool            `json:"autoConcurrency"`
	ConcurrencyUsed        int             `json:"concurrencyUsed"`
	RecommendedConcurrency int             `json:"recommendedConcurrency"`
	LogicalCPU             int             `json:"logicalCPU"`
	PreviewAccounts        []AccountRecord `json:"previewAccounts,omitempty"`
	StoredAccountCount     int             `json:"storedAccountCount,omitempty"`
	AccountsPartial        bool            `json:"accountsPartial,omitempty"`
	Summary                Summary         `json:"summary"`
	Accounts               []AccountRecord `json:"accounts"`
}

type AccountsPageResponse struct {
	ResultID    string          `json:"resultId"`
	Page        int             `json:"page"`
	PageSize    int             `json:"pageSize"`
	Total       int             `json:"total"`
	TotalPages  int             `json:"totalPages"`
	Search      string          `json:"search,omitempty"`
	Status      string          `json:"status,omitempty"`
	Sort        string          `json:"sort,omitempty"`
	OnlyFailure bool            `json:"onlyFailure,omitempty"`
	Items       []AccountRecord `json:"items"`
}

type ScanJob struct {
	ID         string        `json:"id"`
	Status     string        `json:"status"`
	Directory  string        `json:"directory"`
	Done       int           `json:"done"`
	Total      int           `json:"total"`
	Percent    float64       `json:"percent"`
	Message    string        `json:"message,omitempty"`
	StartedAt  string        `json:"startedAt"`
	FinishedAt string        `json:"finishedAt,omitempty"`
	Snapshot   *ScanSnapshot `json:"snapshot,omitempty"`
	finishedAtTime time.Time `json:"-"`
}

type StartJobResponse struct {
	JobID string `json:"jobId"`
}
