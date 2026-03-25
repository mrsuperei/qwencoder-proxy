package token

// Constants
const (
	DefaultQwenBaseURL     = "https://portal.qwen.ai/v1"
	TokenRefreshBufferMs   = 1800 * 1000 // 30 minutes
	QwenOAuthTokenURL      = "https://chat.qwen.ai/api/v1/oauth2/token"
	QwenOAuthClientID      = "f0304373b74a44d2b584a3fb70ca9e56"
	QwenOAuthScope         = "openid profile email model.completion"
	QwenOAuthDeviceAuthURL = "https://chat.qwen.ai/api/v1/oauth2/device/code"

	// Multi-token store constants
	StoreVersion             = 1
	DefaultSelectionStrategy = "random"
	DefaultRefreshBufferSec  = 1800 // 30 minutes
	DefaultMaxErrorCount     = 3
	TokenIDLength            = 16   // UUID length
	CredentialsFileMode      = 0600 // Owner read/write only
	CredentialsDirMode       = 0700 // Owner read/write/execute only

	// User Info URLs for email extraction
	// Note: Qwen's OAuth token response with openid profile email scopes should include email directly
	// If email is not in token response, this endpoint is used as fallback
	QwenUserInfoURL         = "https://chat.qwen.ai/api/v1/user/info"
	GeminiUserInfoURL       = "https://www.googleapis.com/oauth2/v3/userinfo"
	KiroSocialUserInfoURL   = "https://kiro-api.com/api/user/info"
	KiroIdentityUserInfoURL = "https://identity.kiro-api.com/userinfo"
	IFlowUserInfoURL        = "https://iflow.cn/api/oauth/getUserInfo"
)
