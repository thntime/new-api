package controller

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/scrypt"
)

const (
	channelExportFormat  = "new-api-channel-export"
	channelExportVersion = 1
	channelExportAAD     = "new-api-channel-export:v1"

	channelExportKDFName    = "scrypt"
	channelExportCipherName = "aes-256-gcm"

	channelExportSaltSize  = 16
	channelExportScryptN   = 32768
	channelExportScryptR   = 8
	channelExportScryptP   = 1
	channelExportKeyLength = 32

	maxChannelExportChannels       = 1000
	minChannelExportPasswordLength = 8
	maxChannelExportPasswordLength = 128
)

// channelExportSensitiveSettings lists the settings fields stored encrypted
// with the instance CryptoSecret. They are decrypted before export so the file
// can be imported on another instance with a different secret.
var channelExportSensitiveSettings = []string{
	"commandcode_cookie",
	"sensenova_username",
	"sensenova_password",
	"volc_coding_plan_access_key_id",
	"volc_coding_plan_secret_access_key",
}

// errChannelExportInvalid is the single failure message for a wrong password,
// a tampered file and a malformed envelope.
var errChannelExportInvalid = common.NewMessage("Wrong password or corrupted export file")

type channelExportEnvelope struct {
	Format  string              `json:"format"`
	Version int                 `json:"version"`
	KDF     channelExportKDF    `json:"kdf"`
	Cipher  channelExportCipher `json:"cipher"`
}

type channelExportKDF struct {
	Name string `json:"name"`
	N    int    `json:"n"`
	R    int    `json:"r"`
	P    int    `json:"p"`
	Salt string `json:"salt"`
}

type channelExportCipher struct {
	Name  string `json:"name"`
	Nonce string `json:"nonce"`
	Data  string `json:"data"`
}

// channelExportEntry mirrors model.Channel without its id, runtime statistics or
// runtime multi-key state, so future sensitive fields are not exported by default.
type channelExportEntry struct {
	Type               int               `json:"type"`
	Name               string            `json:"name"`
	Key                string            `json:"key"`
	OpenAIOrganization *string           `json:"openai_organization"`
	TestModel          *string           `json:"test_model"`
	Status             int               `json:"status"`
	Weight             *uint             `json:"weight"`
	BaseURL            *string           `json:"base_url"`
	Other              string            `json:"other"`
	Models             string            `json:"models"`
	Group              string            `json:"group"`
	ModelMapping       *string           `json:"model_mapping"`
	StatusCodeMapping  *string           `json:"status_code_mapping"`
	Priority           *int64            `json:"priority"`
	Sort               *int64            `json:"sort"`
	AutoBan            *int              `json:"auto_ban"`
	Tag                *string           `json:"tag"`
	Setting            *string           `json:"setting"`
	ParamOverride      *string           `json:"param_override"`
	HeaderOverride     *string           `json:"header_override"`
	Remark             *string           `json:"remark"`
	Settings           string            `json:"settings"`
	ChannelInfo        channelExportInfo `json:"channel_info"`
}

type channelExportInfo struct {
	IsMultiKey   bool                  `json:"is_multi_key"`
	MultiKeyMode constant.MultiKeyMode `json:"multi_key_mode"`
	IsFallback   bool                  `json:"is_fallback"`
}

func encryptChannelExport(entries []channelExportEntry, password string) ([]byte, error) {
	plaintext, err := common.Marshal(entries)
	if err != nil {
		return nil, err
	}
	salt := make([]byte, channelExportSaltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key, err := scrypt.Key([]byte(password), salt, channelExportScryptN, channelExportScryptR, channelExportScryptP, channelExportKeyLength)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nil, nonce, plaintext, []byte(channelExportAAD))

	envelope := channelExportEnvelope{
		Format:  channelExportFormat,
		Version: channelExportVersion,
		KDF: channelExportKDF{
			Name: channelExportKDFName,
			N:    channelExportScryptN,
			R:    channelExportScryptR,
			P:    channelExportScryptP,
			Salt: base64.StdEncoding.EncodeToString(salt),
		},
		Cipher: channelExportCipher{
			Name:  channelExportCipherName,
			Nonce: base64.StdEncoding.EncodeToString(nonce),
			Data:  base64.StdEncoding.EncodeToString(sealed),
		},
	}
	return common.Marshal(envelope)
}

func decryptChannelExport(content []byte, password string) ([]channelExportEntry, error) {
	var envelope channelExportEnvelope
	if err := common.Unmarshal(content, &envelope); err != nil {
		return nil, errChannelExportInvalid
	}
	if envelope.Format != channelExportFormat ||
		envelope.Version != channelExportVersion ||
		envelope.KDF.Name != channelExportKDFName ||
		envelope.KDF.N != channelExportScryptN ||
		envelope.KDF.R != channelExportScryptR ||
		envelope.KDF.P != channelExportScryptP ||
		envelope.Cipher.Name != channelExportCipherName {
		return nil, errChannelExportInvalid
	}
	salt, err := base64.StdEncoding.DecodeString(envelope.KDF.Salt)
	if err != nil || len(salt) != channelExportSaltSize {
		return nil, errChannelExportInvalid
	}
	nonce, err := base64.StdEncoding.DecodeString(envelope.Cipher.Nonce)
	if err != nil {
		return nil, errChannelExportInvalid
	}
	data, err := base64.StdEncoding.DecodeString(envelope.Cipher.Data)
	if err != nil {
		return nil, errChannelExportInvalid
	}
	key, err := scrypt.Key([]byte(password), salt, channelExportScryptN, channelExportScryptR, channelExportScryptP, channelExportKeyLength)
	if err != nil {
		return nil, errChannelExportInvalid
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errChannelExportInvalid
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errChannelExportInvalid
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, errChannelExportInvalid
	}
	plaintext, err := gcm.Open(nil, nonce, data, []byte(channelExportAAD))
	if err != nil {
		return nil, errChannelExportInvalid
	}
	var entries []channelExportEntry
	if err := common.Unmarshal(plaintext, &entries); err != nil {
		return nil, errChannelExportInvalid
	}
	return entries, nil
}

// buildChannelExportEntry copies only the whitelisted configuration fields and
// restores the settings credentials to plaintext for the encrypted file.
func buildChannelExportEntry(channel *model.Channel) (channelExportEntry, error) {
	entry := channelExportEntry{
		Type:               channel.Type,
		Name:               channel.Name,
		Key:                channel.Key,
		OpenAIOrganization: channel.OpenAIOrganization,
		TestModel:          channel.TestModel,
		Status:             channel.Status,
		Weight:             channel.Weight,
		BaseURL:            channel.BaseURL,
		Other:              channel.Other,
		Models:             channel.Models,
		Group:              channel.Group,
		ModelMapping:       channel.ModelMapping,
		StatusCodeMapping:  channel.StatusCodeMapping,
		Priority:           channel.Priority,
		Sort:               channel.Sort,
		AutoBan:            channel.AutoBan,
		Tag:                channel.Tag,
		Setting:            channel.Setting,
		ParamOverride:      channel.ParamOverride,
		HeaderOverride:     channel.HeaderOverride,
		Remark:             channel.Remark,
		Settings:           channel.OtherSettings,
		ChannelInfo: channelExportInfo{
			IsMultiKey:   channel.ChannelInfo.IsMultiKey,
			MultiKeyMode: channel.ChannelInfo.MultiKeyMode,
			IsFallback:   channel.ChannelInfo.IsFallback,
		},
	}
	if entry.Status == common.ChannelStatusAutoDisabled {
		entry.Status = common.ChannelStatusEnabled
	}
	if entry.Settings != "" {
		settings := map[string]any{}
		if err := common.UnmarshalJsonStr(entry.Settings, &settings); err != nil {
			return channelExportEntry{}, err
		}
		for _, key := range channelExportSensitiveSettings {
			encrypted, ok := settings[key].(string)
			if !ok || encrypted == "" {
				continue
			}
			plain, err := common.DecryptSecret(encrypted)
			if err != nil {
				return channelExportEntry{}, err
			}
			settings[key] = plain
		}
		encoded, err := common.Marshal(settings)
		if err != nil {
			return channelExportEntry{}, err
		}
		entry.Settings = string(encoded)
	}
	return entry, nil
}

// channelFromExportEntry builds a new channel from a file entry, resetting
// runtime state and re-encrypting settings with this instance's CryptoSecret.
func channelFromExportEntry(entry channelExportEntry) (*model.Channel, error) {
	settings := entry.Settings
	if settings != "" {
		merged, err := mergeSensitiveOtherSettings(settings, "")
		if err != nil {
			return nil, err
		}
		settings = merged
	}
	status := entry.Status
	if status == common.ChannelStatusAutoDisabled {
		status = common.ChannelStatusEnabled
	}
	channel := &model.Channel{
		Type:               entry.Type,
		Name:               entry.Name,
		Key:                entry.Key,
		OpenAIOrganization: entry.OpenAIOrganization,
		TestModel:          entry.TestModel,
		Status:             status,
		Weight:             entry.Weight,
		CreatedTime:        common.GetTimestamp(),
		BaseURL:            entry.BaseURL,
		Other:              entry.Other,
		Models:             entry.Models,
		Group:              entry.Group,
		ModelMapping:       entry.ModelMapping,
		StatusCodeMapping:  entry.StatusCodeMapping,
		Priority:           entry.Priority,
		Sort:               entry.Sort,
		AutoBan:            entry.AutoBan,
		Tag:                entry.Tag,
		Setting:            entry.Setting,
		ParamOverride:      entry.ParamOverride,
		HeaderOverride:     entry.HeaderOverride,
		Remark:             entry.Remark,
		OtherSettings:      settings,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   entry.ChannelInfo.IsMultiKey,
			MultiKeySize: channelKeyCount(entry.Key),
			MultiKeyMode: entry.ChannelInfo.MultiKeyMode,
			IsFallback:   entry.ChannelInfo.IsFallback,
		},
	}
	return channel, nil
}

// channelKeyCount counts a channel's keys: a JSON array of keys, or one key per
// line otherwise. A channel without a key has no key count.
func channelKeyCount(key string) int {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return 0
	}
	if strings.HasPrefix(trimmed, "[") {
		var keys []json.RawMessage
		if err := common.Unmarshal([]byte(trimmed), &keys); err == nil {
			return len(keys)
		}
	}
	return len(strings.Split(strings.Trim(key, "\n"), "\n"))
}

type channelExportRequest struct {
	Ids      []int  `json:"ids"`
	Password string `json:"password"`
}

func ExportChannels(c *gin.Context) {
	var request channelExportRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	if len(request.Ids) == 0 {
		common.ApiErrorT(c, "Please select at least one channel")
		return
	}
	if len(request.Ids) > maxChannelExportChannels {
		common.ApiErrorT(c, "You can export at most {{max}} channels at a time", map[string]any{"max": maxChannelExportChannels})
		return
	}
	passwordLength := utf8.RuneCountInString(request.Password)
	if passwordLength < minChannelExportPasswordLength || passwordLength > maxChannelExportPasswordLength {
		common.ApiErrorT(c, "Password must contain between 8 and 128 characters.")
		return
	}

	channels, err := model.GetChannelsByIdsOrdered(request.Ids)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	entries := make([]channelExportEntry, 0, len(channels))
	for _, channel := range channels {
		entry, err := buildChannelExportEntry(channel)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		entries = append(entries, entry)
	}
	content, err := encryptChannelExport(entries, request.Password)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.Header("Cache-Control", "no-store")
	recordManageAudit(c, "channel.export", map[string]any{"count": len(entries)})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"filename": fmt.Sprintf("channels-%s.nacx", time.Now().Format("20060102-150405")),
			"content":  string(content),
		},
	})
}

type channelImportRequest struct {
	Password string `json:"password"`
	Content  string `json:"content"`
}

// channelImportItemError scopes a failed item to its position in the file, so
// the administrator knows which channel to fix.
func channelImportItemError(index int, name string, err error) error {
	params := map[string]any{
		"index": index + 1,
		"name":  name,
		"error": err.Error(),
	}
	// A validation failure is itself a message; carry its key and params so the
	// web console translates the inner text too. error stays for other errors.
	var message *common.Message
	if errors.As(err, &message) {
		params["error_key"] = message.Key
		if len(message.Params) > 0 {
			params["error_params"] = message.Params
		}
	}
	return common.NewMessage("Channel {{index}} ({{name}}) is invalid: {{error}}", params)
}

func ImportChannels(c *gin.Context) {
	var request channelImportRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiError(c, err)
		return
	}
	passwordLength := utf8.RuneCountInString(request.Password)
	if passwordLength < minChannelExportPasswordLength || passwordLength > maxChannelExportPasswordLength {
		common.ApiErrorT(c, "Password must contain between 8 and 128 characters.")
		return
	}

	entries, err := decryptChannelExport([]byte(request.Content), request.Password)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(entries) == 0 {
		common.ApiErrorT(c, "Channel export file is empty")
		return
	}
	if len(entries) > maxChannelExportChannels {
		common.ApiErrorT(c, "You can import at most {{max}} channels at a time", map[string]any{"max": maxChannelExportChannels})
		return
	}

	userID, role := c.GetInt("id"), c.GetInt("role")
	channels := make([]model.Channel, 0, len(entries))
	for index, entry := range entries {
		channel, err := channelFromExportEntry(entry)
		if err != nil {
			common.ApiError(c, channelImportItemError(index, entry.Name, err))
			return
		}
		// Binding a plugin needs the same permission as creating the channel by hand.
		if (channel.Type == constant.ChannelTypeTaskPlugin || len(channel.GetSetting().TaskPluginBindings()) > 0) &&
			!authz.Can(userID, role, authz.TaskPluginBind) {
			common.ApiError(c, channelImportItemError(index, channel.Name, fmt.Errorf("task plugin channels require the task_plugin.bind permission")))
			return
		}
		// checkPluginChannelKeys is not repeated here: it can make one upstream
		// call per key, up to 1000 per import, and the spec's validation list
		// does not include it.
		if err := validateChannel(channel, true); err != nil {
			common.ApiError(c, channelImportItemError(index, channel.Name, err))
			return
		}
		// validateChannel already rejects an empty key; this also catches a key
		// of whitespace only.
		keyless := strings.TrimSpace(channel.Key) == "" && service.ChannelSupportsPluginLogin(channel)
		if !keyless && strings.TrimSpace(channel.Key) == "" {
			common.ApiError(c, channelImportItemError(index, channel.Name, fmt.Errorf("channel cannot be empty")))
			return
		}
		channels = append(channels, *channel)
	}

	if err := model.BatchInsertChannels(channels); err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()

	ids := make([]int, len(channels))
	for index, channel := range channels {
		ids[index] = channel.Id
	}
	recordManageAudit(c, "channel.import", map[string]any{"count": len(channels)})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"count": len(channels),
			"ids":   ids,
		},
	})
}
