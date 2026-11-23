package controller

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// callChannelExportHandler drives an export/import handler through a test
// context without the router and permission middleware.
func callChannelExportHandler(t *testing.T, handler gin.HandlerFunc, userID, role int, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", userID)
	context.Set("role", role)
	context.Set("username", "root")
	context.Request = httptest.NewRequest(http.MethodPost, "/api/channel/export", strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	handler(context)
	return recorder
}

func TestChannelExportImport(t *testing.T) {
	rootID := setupChannelCreateTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.SystemTask{}))
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "channel-export-test-secret"
	t.Cleanup(func() { common.CryptoSecret = previousSecret })

	const password = "correct horse battery staple"

	t.Run("envelope/round-trip", func(t *testing.T) {
		organization := "org-export"
		testModel := "gpt-4o-mini"
		weight := uint(7)
		baseURL := "https://example.com/v1"
		modelMapping := `{"gpt-4o":"gpt-4o-2024-11-20"}`
		statusCodeMapping := `{"500":"503"}`
		priority := int64(10)
		sortOrder := int64(2)
		autoBan := 0
		tag := "export-tag"
		setting := `{"force_format":true}`
		paramOverride := `{"temperature":0}`
		headerOverride := `{"X-Test":"1"}`
		remark := "exported channel"
		entries := []channelExportEntry{
			{
				Type:               1,
				Name:               "round-trip-a",
				Key:                "sk-round-trip-a",
				OpenAIOrganization: &organization,
				TestModel:          &testModel,
				Status:             1,
				Weight:             &weight,
				BaseURL:            &baseURL,
				Other:              `{"default":"us-central1"}`,
				Models:             "gpt-4o,gpt-4o-mini",
				Group:              "default,vip",
				ModelMapping:       &modelMapping,
				StatusCodeMapping:  &statusCodeMapping,
				Priority:           &priority,
				Sort:               &sortOrder,
				AutoBan:            &autoBan,
				Tag:                &tag,
				Setting:            &setting,
				ParamOverride:      &paramOverride,
				HeaderOverride:     &headerOverride,
				Remark:             &remark,
				Settings:           `{"commandcode_cookie":"cookie-value","unknown":"kept"}`,
				ChannelInfo: channelExportInfo{
					IsMultiKey:   true,
					MultiKeyMode: constant.MultiKeyModePolling,
					IsFallback:   true,
				},
			},
			{
				Type:   1,
				Name:   "round-trip-b",
				Key:    "sk-round-trip-b",
				Status: 2,
				Models: "gpt-4o",
				Group:  "default",
			},
		}

		content, err := encryptChannelExport(entries, password)
		require.NoError(t, err)
		require.NotEmpty(t, content)
		assert.NotContains(t, string(content), password)
		assert.NotContains(t, string(content), "sk-round-trip-a")

		decrypted, err := decryptChannelExport(content, password)
		require.NoError(t, err)
		require.Equal(t, entries, decrypted)
	})

	t.Run("envelope/tamper", func(t *testing.T) {
		content, err := encryptChannelExport([]channelExportEntry{{Name: "tamper", Key: "sk-tamper", Type: 1}}, password)
		require.NoError(t, err)

		var envelope channelExportEnvelope
		require.NoError(t, common.Unmarshal(content, &envelope))
		raw, err := base64.StdEncoding.DecodeString(envelope.Cipher.Data)
		require.NoError(t, err)
		require.NotEmpty(t, raw)
		raw[0] ^= 0xff
		envelope.Cipher.Data = base64.StdEncoding.EncodeToString(raw)
		tampered, err := common.Marshal(envelope)
		require.NoError(t, err)

		_, err = decryptChannelExport(tampered, password)
		require.Error(t, err)
		assert.Equal(t, "Wrong password or corrupted export file", err.Error())
	})

	t.Run("export/order-and-fields", func(t *testing.T) {
		seeded := []*model.Channel{
			{Name: "export-order-a", Type: 1, Key: "sk-export-order-a", Models: "gpt-4o", Group: "default", Status: 1},
			{Name: "export-order-b", Type: 1, Key: "sk-export-order-b", Models: "gpt-4o", Group: "default", Status: 2},
			{
				Name: "export-order-c", Type: 1, Key: "sk-export-order-c-1\nsk-export-order-c-2", Models: "gpt-4o", Group: "default", Status: 3,
				ChannelInfo: model.ChannelInfo{
					IsMultiKey:           true,
					MultiKeySize:         2,
					MultiKeyStatusList:   map[int]int{0: 1, 1: 3},
					MultiKeyPollingIndex: 1,
					MultiKeyMode:         constant.MultiKeyModePolling,
					IsFallback:           true,
				},
			},
		}
		for _, channel := range seeded {
			require.NoError(t, channel.Insert())
		}
		ids := []int{seeded[2].Id, seeded[1].Id, seeded[0].Id}
		wantNames := []string{seeded[2].Name, seeded[1].Name, seeded[0].Name}

		body, err := common.Marshal(map[string]any{"ids": ids, "password": password})
		require.NoError(t, err)
		response := callChannelExportHandler(t, ExportChannels, rootID, common.RoleRootUser, string(body))
		require.Equal(t, http.StatusOK, response.Code)
		var result struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
			Data    struct {
				Filename string `json:"filename"`
				Content  string `json:"content"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		require.True(t, result.Success, response.Body.String())
		assert.Empty(t, result.Message)
		assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
		assert.Regexp(t, regexp.MustCompile(`^channels-\d{8}-\d{6}\.nacx$`), result.Data.Filename)

		entries, err := decryptChannelExport([]byte(result.Data.Content), password)
		require.NoError(t, err)
		require.Len(t, entries, 3)
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name)
		}
		assert.Equal(t, wantNames, names, "export order must follow the requested ids")

		plain, err := common.Marshal(entries)
		require.NoError(t, err)
		var rawEntries []map[string]any
		require.NoError(t, common.Unmarshal(plain, &rawEntries))
		require.Len(t, rawEntries, 3)
		for _, raw := range rawEntries {
			for _, excluded := range []string{"id", "balance", "balance_updated_time", "used_quota", "test_time", "response_time", "created_time", "other_info"} {
				assert.NotContains(t, raw, excluded)
			}
			info, ok := raw["channel_info"].(map[string]any)
			require.True(t, ok)
			assert.ElementsMatch(t, []string{"is_multi_key", "multi_key_mode", "is_fallback"}, mapKeys(info))
		}
		assert.Equal(t, 1, entries[0].Status, "automatically disabled channels are exported as enabled")
		assert.Equal(t, 2, entries[1].Status, "manually disabled channels keep their status")
		assert.True(t, entries[0].ChannelInfo.IsMultiKey)
		assert.Equal(t, constant.MultiKeyModePolling, entries[0].ChannelInfo.MultiKeyMode)
		assert.True(t, entries[0].ChannelInfo.IsFallback)
	})

	t.Run("export/sensitive-settings", func(t *testing.T) {
		encryptedCookie, err := common.EncryptSecret("cookie-value")
		require.NoError(t, err)
		settingsJSON, err := common.Marshal(map[string]any{
			"commandcode_cookie": encryptedCookie,
			"unknown_field":      "keep-me",
		})
		require.NoError(t, err)
		channel := &model.Channel{
			Name: "export-sensitive", Type: 1, Key: "sk-export-sensitive", Models: "gpt-4o", Group: "default",
			OtherSettings: string(settingsJSON),
		}
		require.NoError(t, channel.Insert())

		body, err := common.Marshal(map[string]any{"ids": []int{channel.Id}, "password": password})
		require.NoError(t, err)
		response := callChannelExportHandler(t, ExportChannels, rootID, common.RoleRootUser, string(body))
		require.Equal(t, http.StatusOK, response.Code)
		var result struct {
			Success bool `json:"success"`
			Data    struct {
				Content string `json:"content"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		require.True(t, result.Success, response.Body.String())

		entries, err := decryptChannelExport([]byte(result.Data.Content), password)
		require.NoError(t, err)
		require.Len(t, entries, 1)
		var settings map[string]any
		require.NoError(t, common.UnmarshalJsonStr(entries[0].Settings, &settings))
		assert.Equal(t, "cookie-value", settings["commandcode_cookie"])
		assert.Equal(t, "keep-me", settings["unknown_field"])
	})

	t.Run("export/validation", func(t *testing.T) {
		tooManyIds := make([]int, maxChannelExportChannels+1)
		for i := range tooManyIds {
			tooManyIds[i] = i + 1
		}
		tooManyBody, err := common.Marshal(map[string]any{"ids": tooManyIds, "password": password})
		require.NoError(t, err)
		cases := []struct {
			name    string
			body    string
			wantKey string
		}{
			{"empty-ids", `{"ids":[],"password":"` + password + `"}`, "Please select at least one channel"},
			{"too-many", string(tooManyBody), "You can export at most {{max}} channels at a time"},
			{"short-password", `{"ids":[1],"password":"1234567"}`, "Password must contain between 8 and 128 characters."},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				response := callChannelExportHandler(t, ExportChannels, rootID, common.RoleRootUser, tc.body)
				require.Equal(t, http.StatusOK, response.Code)
				var result struct {
					Success    bool   `json:"success"`
					MessageKey string `json:"message_key"`
				}
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
				assert.False(t, result.Success)
				assert.Equal(t, tc.wantKey, result.MessageKey)
			})
		}
	})

	t.Run("export/audit", func(t *testing.T) {
		channel := &model.Channel{Name: "export-audit", Type: 1, Key: "sk-export-audit", Models: "gpt-4o", Group: "default"}
		require.NoError(t, channel.Insert())

		body, err := common.Marshal(map[string]any{"ids": []int{channel.Id}, "password": password})
		require.NoError(t, err)
		response := callChannelExportHandler(t, ExportChannels, rootID, common.RoleRootUser, string(body))
		require.Equal(t, http.StatusOK, response.Code)
		var result struct {
			Success bool `json:"success"`
			Data    struct {
				Filename string `json:"filename"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		require.True(t, result.Success, response.Body.String())

		var audit model.AuditLog
		require.NoError(t, model.LOG_DB.Where("action = ?", "channel.export").Order("id DESC").First(&audit).Error)
		assert.Equal(t, "Exported 1 channels", audit.Content)
		for _, secret := range []string{"sk-export-audit", password, result.Data.Filename} {
			assert.NotContains(t, audit.Content, secret)
		}
	})

	t.Run("import/order-and-fields", func(t *testing.T) {
		priority := int64(5)
		sortOrder := int64(9)
		tag := "import-order"
		setting := `{"force_format":true}`
		baseURL := "https://import-order.example/v1"
		seeded := []*model.Channel{
			{
				Name: "import-order-a", Type: 1, Key: "sk-import-order-a", Models: "gpt-4o", Group: "default",
				Priority: &priority, Sort: &sortOrder, Tag: &tag, Setting: &setting, BaseURL: &baseURL,
				Status: common.ChannelStatusEnabled, Balance: 12.5, UsedQuota: 99, TestTime: 123, ResponseTime: 456,
			},
			{Name: "import-order-b", Type: 1, Key: "sk-import-order-b", Models: "gpt-4o-mini", Group: "vip", Status: common.ChannelStatusManuallyDisabled},
			{Name: "import-order-c", Type: 1, Key: "sk-import-order-c", Models: "gpt-4o", Group: "default", Status: common.ChannelStatusAutoDisabled},
		}
		for _, channel := range seeded {
			require.NoError(t, channel.Insert())
		}

		exportBody, err := common.Marshal(map[string]any{
			"ids":      []int{seeded[0].Id, seeded[1].Id, seeded[2].Id},
			"password": password,
		})
		require.NoError(t, err)
		exported := callChannelExportHandler(t, ExportChannels, rootID, common.RoleRootUser, string(exportBody))
		require.Equal(t, http.StatusOK, exported.Code)
		var exportResult struct {
			Success bool `json:"success"`
			Data    struct {
				Content string `json:"content"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(exported.Body.Bytes(), &exportResult))
		require.True(t, exportResult.Success, exported.Body.String())
		entries, err := decryptChannelExport([]byte(exportResult.Data.Content), password)
		require.NoError(t, err)
		require.Len(t, entries, 3)

		before := common.GetTimestamp()
		response := callChannelExportHandler(t, ImportChannels, rootID, common.RoleRootUser, importChannelsBody(t, exportResult.Data.Content, password))
		require.Equal(t, http.StatusOK, response.Code)
		var result channelImportResponse
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		require.True(t, result.Success, response.Body.String())
		assert.Equal(t, 3, result.Data.Count)
		require.Len(t, result.Data.Ids, 3)
		assert.Less(t, result.Data.Ids[0], result.Data.Ids[1], "imported ids must increase in file order")
		assert.Less(t, result.Data.Ids[1], result.Data.Ids[2], "imported ids must increase in file order")

		stored := make([]*model.Channel, 0, len(result.Data.Ids))
		for index, id := range result.Data.Ids {
			row, err := model.GetChannelById(id, true)
			require.NoError(t, err)
			stored = append(stored, row)
			assert.Equal(t, entries[index].Name, row.Name, "import order must follow the file array")
			assert.Equal(t, entries[index].Type, row.Type)
			assert.Equal(t, entries[index].Key, row.Key)
			assert.Equal(t, entries[index].Models, row.Models)
			assert.Equal(t, entries[index].Group, row.Group)
			assert.Equal(t, entries[index].Status, row.Status)
			assert.GreaterOrEqual(t, row.CreatedTime, before)
			assert.Zero(t, row.Balance)
			assert.Zero(t, row.UsedQuota)
			assert.Zero(t, row.TestTime)
			assert.Zero(t, row.ResponseTime)
			assert.Zero(t, row.BalanceUpdatedTime)
		}
		require.NotNil(t, stored[0].BaseURL)
		assert.Equal(t, baseURL, *stored[0].BaseURL)
		require.NotNil(t, stored[0].Setting)
		assert.Equal(t, setting, *stored[0].Setting)
		require.NotNil(t, stored[0].Priority)
		assert.Equal(t, priority, *stored[0].Priority)
		require.NotNil(t, stored[0].Sort)
		assert.Equal(t, sortOrder, *stored[0].Sort)
		require.NotNil(t, stored[0].Tag)
		assert.Equal(t, tag, *stored[0].Tag)
		assert.Equal(t, common.ChannelStatusManuallyDisabled, stored[1].Status, "a manually disabled channel keeps its status")
		assert.Equal(t, common.ChannelStatusEnabled, stored[2].Status, "an automatically disabled channel is imported as enabled")
	})

	t.Run("import/wrong-password", func(t *testing.T) {
		channel := &model.Channel{Name: "import-wrong-password", Type: 1, Key: "sk-import-wrong-password", Models: "gpt-4o", Group: "default"}
		require.NoError(t, channel.Insert())
		content, err := encryptChannelExport([]channelExportEntry{{
			Type: channel.Type, Name: channel.Name, Key: channel.Key, Models: channel.Models, Group: channel.Group, Status: common.ChannelStatusEnabled,
		}}, password)
		require.NoError(t, err)

		before := channelCount(t)
		response := callChannelExportHandler(t, ImportChannels, rootID, common.RoleRootUser, importChannelsBody(t, string(content), "wrong-password"))
		require.Equal(t, http.StatusOK, response.Code)
		var result channelImportResponse
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		assert.False(t, result.Success)
		assert.Equal(t, "Wrong password or corrupted export file", result.Message)
		assert.Equal(t, before, channelCount(t))
	})

	t.Run("import/tampered", func(t *testing.T) {
		channel := &model.Channel{Name: "import-tampered", Type: 1, Key: "sk-import-tampered", Models: "gpt-4o", Group: "default"}
		require.NoError(t, channel.Insert())
		content, err := encryptChannelExport([]channelExportEntry{{
			Type: channel.Type, Name: channel.Name, Key: channel.Key, Models: channel.Models, Group: channel.Group, Status: common.ChannelStatusEnabled,
		}}, password)
		require.NoError(t, err)

		var envelope channelExportEnvelope
		require.NoError(t, common.Unmarshal(content, &envelope))
		raw, err := base64.StdEncoding.DecodeString(envelope.Cipher.Data)
		require.NoError(t, err)
		require.NotEmpty(t, raw)
		raw[0] ^= 0xff
		envelope.Cipher.Data = base64.StdEncoding.EncodeToString(raw)
		tampered, err := common.Marshal(envelope)
		require.NoError(t, err)

		before := channelCount(t)
		response := callChannelExportHandler(t, ImportChannels, rootID, common.RoleRootUser, importChannelsBody(t, string(tampered), password))
		require.Equal(t, http.StatusOK, response.Code)
		var result channelImportResponse
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		assert.False(t, result.Success)
		assert.Equal(t, "Wrong password or corrupted export file", result.Message)
		assert.Equal(t, before, channelCount(t))
	})

	t.Run("import/invalid-item-rollback", func(t *testing.T) {
		missingSetting := `{"request_plugins":["missing-request-plugin"]}`
		entries := []channelExportEntry{
			{Type: 1, Name: "import-rollback-a", Key: "sk-import-rollback-a", Models: "gpt-4o", Group: "default", Status: common.ChannelStatusEnabled},
			{Type: 1, Name: "import-rollback-b", Key: "sk-import-rollback-b", Models: "gpt-4o", Group: "default", Status: common.ChannelStatusEnabled},
			{Type: 1, Name: "import-rollback-third", Key: "sk-import-rollback-c", Models: "gpt-4o", Group: "default", Status: common.ChannelStatusEnabled, Setting: &missingSetting},
		}
		content, err := encryptChannelExport(entries, password)
		require.NoError(t, err)

		before := channelCount(t)
		response := callChannelExportHandler(t, ImportChannels, rootID, common.RoleRootUser, importChannelsBody(t, string(content), password))
		require.Equal(t, http.StatusOK, response.Code)
		var result channelImportResponse
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		assert.False(t, result.Success)
		assert.Contains(t, result.Message, "3")
		assert.Contains(t, result.Message, "import-rollback-third")
		assert.Contains(t, result.Message, "missing-request-plugin")
		assert.Equal(t, "Request plugin instance {{id}} does not exist", result.MessageParams["error_key"])
		errorParams, ok := result.MessageParams["error_params"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "missing-request-plugin", errorParams["id"])
		assert.Equal(t, before, channelCount(t), "one invalid item must roll back the whole batch")
	})

	t.Run("import/sensitive-settings", func(t *testing.T) {
		encryptedCookie, err := common.EncryptSecret("cookie-value")
		require.NoError(t, err)
		settingsJSON, err := common.Marshal(map[string]any{
			"commandcode_cookie": encryptedCookie,
			"unknown_field":      "keep-me",
		})
		require.NoError(t, err)
		channel := &model.Channel{
			Name: "import-sensitive", Type: 1, Key: "sk-import-sensitive", Models: "gpt-4o", Group: "default",
			OtherSettings: string(settingsJSON),
		}
		require.NoError(t, channel.Insert())

		exportBody, err := common.Marshal(map[string]any{"ids": []int{channel.Id}, "password": password})
		require.NoError(t, err)
		exported := callChannelExportHandler(t, ExportChannels, rootID, common.RoleRootUser, string(exportBody))
		require.Equal(t, http.StatusOK, exported.Code)
		var exportResult struct {
			Success bool `json:"success"`
			Data    struct {
				Content string `json:"content"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(exported.Body.Bytes(), &exportResult))
		require.True(t, exportResult.Success, exported.Body.String())

		response := callChannelExportHandler(t, ImportChannels, rootID, common.RoleRootUser, importChannelsBody(t, exportResult.Data.Content, password))
		require.Equal(t, http.StatusOK, response.Code)
		var result channelImportResponse
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		require.True(t, result.Success, response.Body.String())
		require.Len(t, result.Data.Ids, 1)

		imported, err := model.GetChannelById(result.Data.Ids[0], true)
		require.NoError(t, err)
		var settings map[string]any
		require.NoError(t, common.UnmarshalJsonStr(imported.OtherSettings, &settings))
		storedCookie, ok := settings["commandcode_cookie"].(string)
		require.True(t, ok)
		assert.NotEqual(t, "cookie-value", storedCookie, "the credential is re-encrypted with this instance's secret")
		plain, err := common.DecryptSecret(storedCookie)
		require.NoError(t, err)
		assert.Equal(t, "cookie-value", plain)
		assert.Equal(t, "keep-me", settings["unknown_field"], "unknown settings fields are kept")
	})

	t.Run("import/empty-key", func(t *testing.T) {
		const pluginKey = "import-login"
		source := `
export const meta = {apiVersion: 1, key: "import-login", name: "Import Login", version: "1.0.0", author: {name: "Test"}, models: ["doc"], relay: [{incoming_path: "/v1/chat/completions", upstream_path: "/v1/chat/completions"}]};
export function login(ctx, input) { return {type: "fields", session: {}, fields: []}; }
`
		_, err := jsplugin.DefaultRegistry.Register(source, jsplugin.Options{})
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, jsplugin.DefaultRegistry.Unregister(pluginKey)) })

		pluginSetting := fmt.Sprintf(`{"task_plugin_key":%q}`, pluginKey)
		pluginBaseURL := "https://import-login.example"
		keylessContent, err := encryptChannelExport([]channelExportEntry{{
			Type: constant.ChannelTypeTaskPlugin, Name: "import-keyless-plugin", Key: "", Models: "doc", Group: "default",
			Status: common.ChannelStatusEnabled, BaseURL: &pluginBaseURL, Setting: &pluginSetting,
		}}, password)
		require.NoError(t, err)

		response := callChannelExportHandler(t, ImportChannels, rootID, common.RoleRootUser, importChannelsBody(t, string(keylessContent), password))
		require.Equal(t, http.StatusOK, response.Code)
		var result channelImportResponse
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		require.True(t, result.Success, response.Body.String())
		require.Len(t, result.Data.Ids, 1)
		imported, err := model.GetChannelById(result.Data.Ids[0], true)
		require.NoError(t, err)
		assert.Empty(t, imported.Key)
		assert.Zero(t, imported.ChannelInfo.MultiKeySize, "a keyless channel has no multi-key size")

		plainContent, err := encryptChannelExport([]channelExportEntry{{
			Type: 1, Name: "import-empty-key", Key: "", Models: "gpt-4o", Group: "default", Status: common.ChannelStatusEnabled,
		}}, password)
		require.NoError(t, err)
		before := channelCount(t)
		rejected := callChannelExportHandler(t, ImportChannels, rootID, common.RoleRootUser, importChannelsBody(t, string(plainContent), password))
		require.Equal(t, http.StatusOK, rejected.Code)
		var rejectedResult channelImportResponse
		require.NoError(t, common.Unmarshal(rejected.Body.Bytes(), &rejectedResult))
		assert.False(t, rejectedResult.Success)
		assert.Contains(t, rejectedResult.Message, "1")
		assert.Contains(t, rejectedResult.Message, "import-empty-key")
		assert.Contains(t, rejectedResult.Message, "channel cannot be empty")
		assert.Equal(t, before, channelCount(t), "a keyless non-plugin channel must not be inserted")
	})

	t.Run("import/empty-file", func(t *testing.T) {
		content, err := encryptChannelExport([]channelExportEntry{}, password)
		require.NoError(t, err)

		before := channelCount(t)
		response := callChannelExportHandler(t, ImportChannels, rootID, common.RoleRootUser, importChannelsBody(t, string(content), password))
		require.Equal(t, http.StatusOK, response.Code)
		var result channelImportResponse
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		assert.False(t, result.Success)
		assert.Equal(t, "Channel export file is empty", result.MessageKey)
		assert.Equal(t, before, channelCount(t))
	})

	t.Run("import/too-many", func(t *testing.T) {
		entries := make([]channelExportEntry, maxChannelExportChannels+1)
		for index := range entries {
			entries[index] = channelExportEntry{
				Type: 1, Name: fmt.Sprintf("import-too-many-%d", index+1), Key: "sk-import-too-many", Models: "gpt-4o",
				Group: "default", Status: common.ChannelStatusEnabled,
			}
		}
		content, err := encryptChannelExport(entries, password)
		require.NoError(t, err)

		before := channelCount(t)
		response := callChannelExportHandler(t, ImportChannels, rootID, common.RoleRootUser, importChannelsBody(t, string(content), password))
		require.Equal(t, http.StatusOK, response.Code)
		var result channelImportResponse
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		assert.False(t, result.Success)
		assert.Equal(t, "You can import at most {{max}} channels at a time", result.MessageKey)
		assert.Equal(t, before, channelCount(t))
	})

	t.Run("import/audit", func(t *testing.T) {
		content, err := encryptChannelExport([]channelExportEntry{
			{Type: 1, Name: "import-audit-a", Key: "sk-import-audit-a", Models: "gpt-4o", Group: "default", Status: common.ChannelStatusEnabled},
			{Type: 1, Name: "import-audit-b", Key: "sk-import-audit-b", Models: "gpt-4o", Group: "default", Status: common.ChannelStatusEnabled},
		}, password)
		require.NoError(t, err)

		response := callChannelExportHandler(t, ImportChannels, rootID, common.RoleRootUser, importChannelsBody(t, string(content), password))
		require.Equal(t, http.StatusOK, response.Code)
		var result channelImportResponse
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		require.True(t, result.Success, response.Body.String())

		var audit model.AuditLog
		require.NoError(t, model.LOG_DB.Where("action = ?", "channel.import").Order("id DESC").First(&audit).Error)
		assert.Equal(t, "Imported 2 channels", audit.Content)
		for _, secret := range []string{"sk-import-audit-a", "sk-import-audit-b", password} {
			assert.NotContains(t, audit.Content, secret)
		}
	})
}

type channelImportResponse struct {
	Success       bool           `json:"success"`
	Message       string         `json:"message"`
	MessageKey    string         `json:"message_key"`
	MessageParams map[string]any `json:"message_params"`
	Data          struct {
		Count int   `json:"count"`
		Ids   []int `json:"ids"`
	} `json:"data"`
}

func importChannelsBody(t *testing.T, content, password string) string {
	t.Helper()
	body, err := common.Marshal(map[string]any{"password": password, "content": content})
	require.NoError(t, err)
	return string(body)
}

func channelCount(t *testing.T) int64 {
	t.Helper()
	var count int64
	require.NoError(t, model.DB.Model(&model.Channel{}).Count(&count).Error)
	return count
}

func mapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
