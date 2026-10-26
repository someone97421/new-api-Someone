package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedFlowQuotaData(t *testing.T, quotaData QuotaData) {
	t.Helper()
	require.NoError(t, DB.Create(&quotaData).Error)
}

func seedFlowLookupData(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.Create(&Channel{Id: 1, Name: "east"}).Error)
	require.NoError(t, DB.Create(&Channel{Id: 2, Name: "west"}).Error)
	require.NoError(t, DB.Create(&Token{Id: 11, UserId: 1, Key: "sk-primary", Name: "primary"}).Error)
	require.NoError(t, DB.Create(&Token{Id: 22, UserId: 2, Key: "sk-backup", Name: "backup"}).Error)
	require.NoError(t, DB.Delete(&Token{Id: 11}).Error)
}

func TestGetFlowQuotaDataUsesQuotaDataRoleSpecificDimensions(t *testing.T) {
	truncateTables(t)
	seedFlowLookupData(t)

	seedFlowQuotaData(t, QuotaData{
		UserID:    1,
		Username:  "alice",
		NodeName:  "node-a",
		TokenID:   11,
		UseGroup:  "vip",
		ModelName: "gpt-a",
		ChannelID: 1,
		CreatedAt: 1000,
		Count:     2,
		Quota:     100,
		TokenUsed: 40,
	})
	seedFlowQuotaData(t, QuotaData{
		UserID:    1,
		Username:  "alice",
		NodeName:  "node-a",
		TokenID:   11,
		UseGroup:  "vip",
		ModelName: "gpt-a",
		ChannelID: 1,
		CreatedAt: 1100,
		Count:     1,
		Quota:     50,
		TokenUsed: 20,
	})
	seedFlowQuotaData(t, QuotaData{
		UserID:    1,
		Username:  "alice",
		NodeName:  "node-a",
		TokenID:   11,
		UseGroup:  "vip",
		ModelName: "gpt-a",
		ChannelID: 2,
		CreatedAt: 1200,
		Count:     1,
		Quota:     25,
		TokenUsed: 10,
	})
	seedFlowQuotaData(t, QuotaData{
		UserID:    2,
		Username:  "bob",
		NodeName:  "node-b",
		TokenID:   22,
		UseGroup:  "default",
		ModelName: "gpt-b",
		ChannelID: 1,
		CreatedAt: 1300,
		Count:     3,
		Quota:     70,
		TokenUsed: 30,
	})
	seedFlowQuotaData(t, QuotaData{
		UserID:    1,
		Username:  "alice",
		ModelName: "legacy",
		CreatedAt: 1400,
		Count:     99,
		Quota:     999,
		TokenUsed: 999,
	})

	rootRows, err := GetFlowQuotaData(900, 2000, "", 0, common.RoleRootUser)
	require.NoError(t, err)
	require.Len(t, rootRows, 3)
	// Token 11 was soft-deleted, so its name is intentionally left empty for the
	// frontend to render a localized "deleted (id)" label instead.
	require.Equal(t, FlowQuotaData{
		UserID:      1,
		Username:    "alice",
		NodeName:    "node-a",
		TokenID:     11,
		TokenName:   "",
		UseGroup:    "vip",
		ChannelID:   1,
		ChannelName: "east",
		ModelName:   "gpt-a",
		TokenUsed:   60,
		Count:       3,
		Quota:       150,
	}, *rootRows[0])
	// A token that still exists resolves to its current name.
	require.Equal(t, 22, rootRows[1].TokenID)
	require.Equal(t, "backup", rootRows[1].TokenName)

	adminRows, err := GetFlowQuotaData(900, 2000, "alice", 0, common.RoleAdminUser)
	require.NoError(t, err)
	require.Len(t, adminRows, 2)
	require.Equal(t, 0, adminRows[0].TokenID)
	require.Empty(t, adminRows[0].TokenName)
	require.Empty(t, adminRows[0].NodeName)
	require.Equal(t, "alice", adminRows[0].Username)
	require.Equal(t, "vip", adminRows[0].UseGroup)
	require.Equal(t, "east", adminRows[0].ChannelName)
	require.Equal(t, 150, adminRows[0].Quota)

	selfRows, err := GetFlowQuotaData(900, 2000, "", 1, common.RoleCommonUser)
	require.NoError(t, err)
	require.Len(t, selfRows, 1)
	require.Empty(t, selfRows[0].Username)
	require.Equal(t, 0, selfRows[0].ChannelID)
	require.Empty(t, selfRows[0].ChannelName)
	require.Empty(t, selfRows[0].TokenName)
	require.Equal(t, "vip", selfRows[0].UseGroup)
	require.Equal(t, 175, selfRows[0].Quota)
}

func TestLogQuotaDataSplitsRowsByUseGroupTokenChannelAndNode(t *testing.T) {
	truncateTables(t)
	CacheQuotaDataLock.Lock()
	CacheQuotaData = make(map[string]*QuotaData)
	CacheQuotaDataLock.Unlock()

	LogQuotaData(QuotaDataLogParams{
		UserID:    1,
		Username:  "alice",
		ModelName: "gpt-a",
		CreatedAt: 3661,
		UseGroup:  "vip",
		TokenID:   11,
		ChannelID: 1,
		NodeName:  "node-a",
		Quota:     100,
		TokenUsed: 40,
	})
	LogQuotaData(QuotaDataLogParams{
		UserID:    1,
		Username:  "alice",
		ModelName: "gpt-a",
		CreatedAt: 3700,
		UseGroup:  "vip",
		TokenID:   11,
		ChannelID: 1,
		NodeName:  "node-a",
		Quota:     50,
		TokenUsed: 20,
	})
	LogQuotaData(QuotaDataLogParams{
		UserID:    1,
		Username:  "alice",
		ModelName: "gpt-a",
		CreatedAt: 3700,
		UseGroup:  "default",
		TokenID:   11,
		ChannelID: 1,
		NodeName:  "node-a",
		Quota:     25,
		TokenUsed: 10,
	})

	SaveQuotaDataCache()

	var rows []QuotaData
	require.NoError(t, DB.Order("quota DESC").Find(&rows).Error)
	require.Len(t, rows, 2)
	require.Equal(t, int64(3600), rows[0].CreatedAt)
	require.Equal(t, "vip", rows[0].UseGroup)
	require.Equal(t, 11, rows[0].TokenID)
	require.Equal(t, 1, rows[0].ChannelID)
	require.Equal(t, "node-a", rows[0].NodeName)
	require.Equal(t, 2, rows[0].Count)
	require.Equal(t, 150, rows[0].Quota)
	require.Equal(t, 60, rows[0].TokenUsed)
	require.Equal(t, "default", rows[1].UseGroup)
	require.Equal(t, 25, rows[1].Quota)
}

// 合入缓存统计前（32e587406）的真实 quota_data 结构。
type quotaDataBeforeCache struct {
	Id        int
	UserID    int    `gorm:"index"`
	Username  string `gorm:"index:idx_qdt_model_user_name,priority:2;size:64;default:''"`
	ModelName string `gorm:"index:idx_qdt_model_user_name,priority:1;size:64;default:''"`
	CreatedAt int64  `gorm:"bigint;index:idx_qdt_created_at,priority:2"`
	UseGroup  string `gorm:"index;size:64;default:''"`
	TokenID   int    `gorm:"index;default:0"`
	ChannelID int    `gorm:"index;default:0"`
	NodeName  string `gorm:"index;size:64;default:''"`
	TokenUsed int    `gorm:"default:0"`
	Count     int    `gorm:"default:0"`
	Quota     int    `gorm:"default:0"`
}

func TestQuotaDataCacheMigrationDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var dsn string
			switch dialect {
			case "sqlite":
				dsn = "local"
				previousPath := common.SQLitePath
				common.SQLitePath = filepath.Join(t.TempDir(), "quota-cache.db")
				t.Cleanup(func() { common.SQLitePath = previousPath })
			case "mysql":
				dsn = os.Getenv("TEST_MYSQL_DSN")
			case "postgres":
				dsn = os.Getenv("TEST_POSTGRES_DSN")
			}
			if dsn == "" {
				t.Skip("test database DSN is not configured")
			}
			t.Setenv("QUOTA_CACHE_TEST_DSN", dsn)
			db, dbType, err := chooseDB("QUOTA_CACHE_TEST_DSN", false)
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			previousDB, previousType := DB, common.MainDatabaseType()
			previousCache := CacheQuotaData
			t.Cleanup(func() {
				DB = previousDB
				common.SetMainDatabaseType(previousType)
				CacheQuotaData = previousCache
			})
			common.SetMainDatabaseType(dbType)
			recorder := &migrationSQLRecorder{}
			DB = db.Session(&gorm.Session{Logger: recorder})
			var version string
			versionQuery := "SELECT VERSION()"
			if dialect == "sqlite" {
				versionQuery = "SELECT sqlite_version()"
			}
			require.NoError(t, DB.Raw(versionQuery).Scan(&version).Error)
			t.Logf("database version: %s", version)
			for _, baseline := range []string{"fresh", "before_cache_columns", "release_rc47"} {
				t.Run(baseline, func(t *testing.T) {
					require.NoError(t, DB.Migrator().DropTable(&QuotaData{}))
					t.Cleanup(func() { _ = DB.Migrator().DropTable(&QuotaData{}) })
					CacheQuotaData = make(map[string]*QuotaData)
					if baseline == "before_cache_columns" {
						require.NoError(t, DB.Table("quota_data").AutoMigrate(&quotaDataBeforeCache{}))
						require.NoError(t, DB.Table("quota_data").Create(&quotaDataBeforeCache{Id: 7, Username: "legacy", Quota: 17, Count: 1}).Error)
					} else if baseline == "release_rc47" {
						// v1.0.0-rc.47 的 QuotaData 结构与本次合并后的结构相同。
						require.NoError(t, DB.AutoMigrate(&QuotaData{}))
						require.NoError(t, DB.Create(&QuotaData{Id: 7, Username: "legacy", Quota: 17, Count: 1, CacheReadTokens: 2, CacheCreationTokens: 3}).Error)
					}
					require.NoError(t, DB.AutoMigrate(&QuotaData{}))
					recorder.reset()
					require.NoError(t, DB.AutoMigrate(&QuotaData{}))
					assert.Empty(t, recorder.schemaMutations(), "重复迁移不得改变结构")
					for _, index := range []string{"idx_quota_data_user_id", "idx_qdt_model_user_name", "idx_qdt_created_at", "idx_quota_data_use_group", "idx_quota_data_token_id", "idx_quota_data_channel_id", "idx_quota_data_node_name"} {
						assert.True(t, DB.Migrator().HasIndex(&QuotaData{}, index), index)
					}
					if baseline != "fresh" {
						var saved QuotaData
						require.NoError(t, DB.First(&saved, 7).Error)
						assert.Equal(t, "legacy", saved.Username)
						assert.Equal(t, 17, saved.Quota)
						assert.Equal(t, 1, saved.Count)
						if baseline == "before_cache_columns" {
							assert.Zero(t, saved.CacheReadTokens)
							assert.Zero(t, saved.CacheCreationTokens)
						} else {
							assert.EqualValues(t, 2, saved.CacheReadTokens)
							assert.EqualValues(t, 3, saved.CacheCreationTokens)
						}
						assert.Error(t, DB.Create(&QuotaData{Id: 7}).Error, "保留主键唯一约束")
					}
					params := QuotaDataLogParams{UserID: 8, Username: "cache-user", ModelName: "cache-model", CreatedAt: 3661, TokenUsed: 20, CacheReadTokens: 11, CacheCreationTokens: 7}
					LogQuotaData(params)
					LogQuotaData(params)
					SaveQuotaDataCache()
					params.TokenUsed, params.CacheReadTokens, params.CacheCreationTokens = 10, 3, 2
					LogQuotaData(params)
					SaveQuotaDataCache()
					byName, err := GetQuotaDataByUsername("cache-user", 3500, 4000)
					require.NoError(t, err)
					byUser, err := GetQuotaDataByUserId(8, 3500, 4000)
					require.NoError(t, err)
					byGroup, err := GetQuotaDataGroupByUser(3500, 4000)
					require.NoError(t, err)
					all, err := GetAllQuotaDates(3500, 4000, "")
					require.NoError(t, err)
					for _, rows := range [][]*QuotaData{byName, byUser, byGroup, all} {
						require.Len(t, rows, 1)
						assert.Equal(t, 3, rows[0].Count)
						assert.Equal(t, 50, rows[0].TokenUsed)
						assert.EqualValues(t, 25, rows[0].CacheReadTokens)
						assert.EqualValues(t, 16, rows[0].CacheCreationTokens)
					}
				})
			}
		})
	}
}

func (quotaDataBeforeCache) TableName() string { return "quota_data" }
