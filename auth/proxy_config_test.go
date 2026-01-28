// Package auth provides authentication and token management functionality.
package auth

import (
	"encoding/json"
	"testing"
)

func TestProxyTypeValidate(t *testing.T) {
	tests := []struct {
		name    string
		pt      ProxyType
		wantErr bool
	}{
		{"Valid none", ProxyTypeNone, false},
		{"Valid http", ProxyTypeHTTP, false},
		{"Valid https", ProxyTypeHTTPS, false},
		{"Valid socks5", ProxyTypeSOCKS5, false},
		{"Invalid type", ProxyType("invalid"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pt.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ProxyType.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestProxyTypeString(t *testing.T) {
	tests := []struct {
		name string
		pt   ProxyType
		want string
	}{
		{"None", ProxyTypeNone, "none"},
		{"HTTP", ProxyTypeHTTP, "http"},
		{"HTTPS", ProxyTypeHTTPS, "https"},
		{"SOCKS5", ProxyTypeSOCKS5, "socks5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pt.String(); got != tt.want {
				t.Errorf("ProxyType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProxyConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		pc      *ProxyConfig
		wantErr bool
	}{
		{
			name:    "Nil config is valid",
			pc:      nil,
			wantErr: false,
		},
		{
			name: "Valid none type",
			pc: &ProxyConfig{
				Type:    ProxyTypeNone,
				Enabled: false,
			},
			wantErr: false,
		},
		{
			name: "Valid HTTP proxy",
			pc: &ProxyConfig{
				Type:    ProxyTypeHTTP,
				Host:    "proxy.example.com",
				Port:    8080,
				Enabled: true,
			},
			wantErr: false,
		},
		{
			name: "Valid HTTPS proxy with auth",
			pc: &ProxyConfig{
				Type:     ProxyTypeHTTPS,
				Host:     "secure.proxy.com",
				Port:     443,
				Username: "user",
				Password: "pass",
				Enabled:  true,
			},
			wantErr: false,
		},
		{
			name: "Valid SOCKS5 proxy",
			pc: &ProxyConfig{
				Type:    ProxyTypeSOCKS5,
				Host:    "socks5.example.com",
				Port:    1080,
				Enabled: true,
			},
			wantErr: false,
		},
		{
			name: "Invalid proxy type",
			pc: &ProxyConfig{
				Type:    ProxyType("invalid"),
				Host:    "proxy.example.com",
				Port:    8080,
				Enabled: true,
			},
			wantErr: true,
		},
		{
			name: "Empty host for non-none type",
			pc: &ProxyConfig{
				Type:    ProxyTypeHTTP,
				Host:    "",
				Port:    8080,
				Enabled: true,
			},
			wantErr: true,
		},
		{
			name: "Port out of range - negative",
			pc: &ProxyConfig{
				Type:    ProxyTypeHTTP,
				Host:    "proxy.example.com",
				Port:    -1,
				Enabled: true,
			},
			wantErr: true,
		},
		{
			name: "Port out of range - too high",
			pc: &ProxyConfig{
				Type:    ProxyTypeHTTP,
				Host:    "proxy.example.com",
				Port:    70000,
				Enabled: true,
			},
			wantErr: true,
		},
		{
			name: "Port zero",
			pc: &ProxyConfig{
				Type:    ProxyTypeHTTP,
				Host:    "proxy.example.com",
				Port:    0,
				Enabled: true,
			},
			wantErr: true,
		},
		{
			name: "Username without password",
			pc: &ProxyConfig{
				Type:     ProxyTypeHTTP,
				Host:     "proxy.example.com",
				Port:     8080,
				Username: "user",
				Enabled:  true,
			},
			wantErr: true,
		},
		{
			name: "Password without username",
			pc: &ProxyConfig{
				Type:     ProxyTypeHTTP,
				Host:     "proxy.example.com",
				Port:     8080,
				Password: "pass",
				Enabled:  true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pc.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ProxyConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestProxyConfigString(t *testing.T) {
	tests := []struct {
		name string
		pc   *ProxyConfig
		want string
	}{
		{
			name: "Nil config",
			pc:   nil,
			want: "ProxyConfig(nil)",
		},
		{
			name: "Config without password",
			pc: &ProxyConfig{
				Type:    ProxyTypeHTTP,
				Host:    "proxy.example.com",
				Port:    8080,
				Enabled: true,
			},
			want: "ProxyConfig{Type: http, Host: proxy.example.com, Port: 8080, Username: , Password: , Enabled: true}",
		},
		{
			name: "Config with password (masked)",
			pc: &ProxyConfig{
				Type:     ProxyTypeHTTPS,
				Host:     "secure.proxy.com",
				Port:     443,
				Username: "user",
				Password: "secret",
				Enabled:  true,
			},
			want: "ProxyConfig{Type: https, Host: secure.proxy.com, Port: 443, Username: user, Password: ****, Enabled: true}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pc.String(); got != tt.want {
				t.Errorf("ProxyConfig.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProxyConfigMaskedPassword(t *testing.T) {
	tests := []struct {
		name string
		pc   *ProxyConfig
		want string
	}{
		{
			name: "Nil config",
			pc:   nil,
			want: "",
		},
		{
			name: "Config without password",
			pc: &ProxyConfig{
				Type:    ProxyTypeHTTP,
				Host:    "proxy.example.com",
				Port:    8080,
				Enabled: true,
			},
			want: "",
		},
		{
			name: "Config with password",
			pc: &ProxyConfig{
				Type:     ProxyTypeHTTPS,
				Host:     "secure.proxy.com",
				Port:     443,
				Username: "user",
				Password: "secret",
				Enabled:  true,
			},
			want: "******",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pc.MaskedPassword(); got != tt.want {
				t.Errorf("ProxyConfig.MaskedPassword() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProxyConfigKeyString(t *testing.T) {
	tests := []struct {
		name string
		pck  ProxyConfigKey
		want string
	}{
		{
			name: "HTTP proxy key",
			pck: ProxyConfigKey{
				Type: ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 8080,
			},
			want: "http:proxy.example.com:8080",
		},
		{
			name: "SOCKS5 proxy key",
			pck: ProxyConfigKey{
				Type: ProxyTypeSOCKS5,
				Host: "socks.example.com",
				Port: 1080,
			},
			want: "socks5:socks.example.com:1080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pck.String(); got != tt.want {
				t.Errorf("ProxyConfigKey.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProxyConfigKeyMarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		pck     ProxyConfigKey
		want    string
		wantErr bool
	}{
		{
			name: "Marshal HTTP proxy key",
			pck: ProxyConfigKey{
				Type: ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 8080,
			},
			want:    `"http:proxy.example.com:8080"`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.pck)
			if (err != nil) != tt.wantErr {
				t.Errorf("ProxyConfigKey.MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if string(got) != tt.want {
				t.Errorf("ProxyConfigKey.MarshalJSON() = %v, want %v", string(got), tt.want)
			}
		})
	}
}

func TestProxyConfigKeyUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    ProxyConfigKey
		wantErr bool
	}{
		{
			name: "Unmarshal HTTP proxy key",
			data: `"http:proxy.example.com:8080"`,
			want: ProxyConfigKey{
				Type: ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 8080,
			},
			wantErr: false,
		},
		{
			name: "Unmarshal SOCKS5 proxy key",
			data: `"socks5:socks.example.com:1080"`,
			want: ProxyConfigKey{
				Type: ProxyTypeSOCKS5,
				Host: "socks.example.com",
				Port: 1080,
			},
			wantErr: false,
		},
		{
			name:    "Invalid format - missing parts",
			data:    `"http:proxy.example.com"`,
			wantErr: true,
		},
		{
			name:    "Invalid format - too many parts",
			data:    `"http:proxy.example.com:8080:extra"`,
			wantErr: true,
		},
		{
			name:    "Invalid port",
			data:    `"http:proxy.example.com:invalid"`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got ProxyConfigKey
			err := json.Unmarshal([]byte(tt.data), &got)
			if (err != nil) != tt.wantErr {
				t.Errorf("ProxyConfigKey.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ProxyConfigKey.UnmarshalJSON() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewProxyConfigKey(t *testing.T) {
	tests := []struct {
		name string
		pc   *ProxyConfig
		want *ProxyConfigKey
	}{
		{
			name: "Nil config returns nil",
			pc:   nil,
			want: nil,
		},
		{
			name: "None type returns nil",
			pc: &ProxyConfig{
				Type:    ProxyTypeNone,
				Enabled: false,
			},
			want: nil,
		},
		{
			name: "HTTP proxy returns key",
			pc: &ProxyConfig{
				Type:     ProxyTypeHTTP,
				Host:     "proxy.example.com",
				Port:     8080,
				Username: "user",
				Password: "pass",
				Enabled:  true,
			},
			want: &ProxyConfigKey{
				Type: ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 8080,
			},
		},
		{
			name: "SOCKS5 proxy returns key",
			pc: &ProxyConfig{
				Type:    ProxyTypeSOCKS5,
				Host:    "socks.example.com",
				Port:    1080,
				Enabled: true,
			},
			want: &ProxyConfigKey{
				Type: ProxyTypeSOCKS5,
				Host: "socks.example.com",
				Port: 1080,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewProxyConfigKey(tt.pc)
			if got == nil && tt.want == nil {
				return
			}
			if (got == nil) != (tt.want == nil) {
				t.Errorf("NewProxyConfigKey() = %v, want %v", got, tt.want)
				return
			}
			if *got != *tt.want {
				t.Errorf("NewProxyConfigKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProxyConfigJSONMarshaling(t *testing.T) {
	tests := []struct {
		name    string
		pc      *ProxyConfig
		want    string
		wantErr bool
	}{
		{
			name: "Marshal config with all fields",
			pc: &ProxyConfig{
				Type:     ProxyTypeHTTPS,
				Host:     "secure.proxy.com",
				Port:     443,
				Username: "user",
				Password: "secret",
				Enabled:  true,
			},
			want:    `{"type":"https","host":"secure.proxy.com","port":443,"username":"user","password":"secret","enabled":true}`,
			wantErr: false,
		},
		{
			name: "Marshal config without auth",
			pc: &ProxyConfig{
				Type:    ProxyTypeHTTP,
				Host:    "proxy.example.com",
				Port:    8080,
				Enabled: true,
			},
			want:    `{"type":"http","host":"proxy.example.com","port":8080,"enabled":true}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.pc)
			if (err != nil) != tt.wantErr {
				t.Errorf("json.Marshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && string(got) != tt.want {
				t.Errorf("json.Marshal() = %v, want %v", string(got), tt.want)
			}
		})
	}
}

func TestProxyConfigJSONUnmarshaling(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    *ProxyConfig
		wantErr bool
	}{
		{
			name: "Unmarshal full config",
			data: `{"type":"https","host":"secure.proxy.com","port":443,"username":"user","password":"secret","enabled":true}`,
			want: &ProxyConfig{
				Type:     ProxyTypeHTTPS,
				Host:     "secure.proxy.com",
				Port:     443,
				Username: "user",
				Password: "secret",
				Enabled:  true,
			},
			wantErr: false,
		},
		{
			name: "Unmarshal config without auth",
			data: `{"type":"http","host":"proxy.example.com","port":8080,"enabled":true}`,
			want: &ProxyConfig{
				Type:    ProxyTypeHTTP,
				Host:    "proxy.example.com",
				Port:    8080,
				Enabled: true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got ProxyConfig
			err := json.Unmarshal([]byte(tt.data), &got)
			if (err != nil) != tt.wantErr {
				t.Errorf("json.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != *tt.want {
				t.Errorf("json.Unmarshal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidationErrorMessages(t *testing.T) {
	tests := []struct {
		name       string
		pc         *ProxyConfig
		wantErrMsg string
	}{
		{
			name: "Invalid proxy type error message",
			pc: &ProxyConfig{
				Type:    ProxyType("invalid"),
				Host:    "proxy.example.com",
				Port:    8080,
				Enabled: true,
			},
			wantErrMsg: "invalid proxy type: invalid",
		},
		{
			name: "Empty host error message",
			pc: &ProxyConfig{
				Type:    ProxyTypeHTTP,
				Host:    "",
				Port:    8080,
				Enabled: true,
			},
			wantErrMsg: "proxy host cannot be empty for type http",
		},
		{
			name: "Port out of range error message",
			pc: &ProxyConfig{
				Type:    ProxyTypeHTTP,
				Host:    "proxy.example.com",
				Port:    70000,
				Enabled: true,
			},
			wantErrMsg: "proxy port must be between 1 and 65535, got: 70000",
		},
		{
			name: "Username without password error message",
			pc: &ProxyConfig{
				Type:     ProxyTypeHTTP,
				Host:     "proxy.example.com",
				Port:     8080,
				Username: "user",
				Enabled:  true,
			},
			wantErrMsg: "both username and password must be provided together, or neither",
		},
		{
			name: "Password without username error message",
			pc: &ProxyConfig{
				Type:     ProxyTypeHTTP,
				Host:     "proxy.example.com",
				Port:     8080,
				Password: "pass",
				Enabled:  true,
			},
			wantErrMsg: "both username and password must be provided together, or neither",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pc.Validate()
			if err == nil {
				t.Errorf("Expected error containing %q, got nil", tt.wantErrMsg)
				return
			}
			if err.Error() != tt.wantErrMsg {
				t.Errorf("Error message = %q, want %q", err.Error(), tt.wantErrMsg)
			}
		})
	}
}

func TestKeyEquality(t *testing.T) {
	tests := []struct {
		name string
		key1 ProxyConfigKey
		key2 ProxyConfigKey
		want bool
	}{
		{
			name: "Same keys are equal",
			key1: ProxyConfigKey{
				Type: ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 8080,
			},
			key2: ProxyConfigKey{
				Type: ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 8080,
			},
			want: true,
		},
		{
			name: "Different type - not equal",
			key1: ProxyConfigKey{
				Type: ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 8080,
			},
			key2: ProxyConfigKey{
				Type: ProxyTypeHTTPS,
				Host: "proxy.example.com",
				Port: 8080,
			},
			want: false,
		},
		{
			name: "Different host - not equal",
			key1: ProxyConfigKey{
				Type: ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 8080,
			},
			key2: ProxyConfigKey{
				Type: ProxyTypeHTTP,
				Host: "other.proxy.com",
				Port: 8080,
			},
			want: false,
		},
		{
			name: "Different port - not equal",
			key1: ProxyConfigKey{
				Type: ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 8080,
			},
			key2: ProxyConfigKey{
				Type: ProxyTypeHTTP,
				Host: "proxy.example.com",
				Port: 9090,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.key1 == tt.key2
			if got != tt.want {
				t.Errorf("Key equality = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKeyInequality(t *testing.T) {
	key1 := ProxyConfigKey{
		Type: ProxyTypeHTTP,
		Host: "proxy.example.com",
		Port: 8080,
	}
	key2 := ProxyConfigKey{
		Type: ProxyTypeHTTPS,
		Host: "proxy.example.com",
		Port: 8080,
	}

	if key1 == key2 {
		t.Error("Expected keys to be unequal")
	}
}

func TestBackwardCompatibilityMissingProxy(t *testing.T) {
	// Simulate an old token structure without proxy fields
	oldTokenJSON := `{"id":"token123","access_token":"abc123"}`

	// Unmarshal into a structure that has proxy fields
	type TokenWithProxy struct {
		ID          string       `json:"id"`
		AccessToken string       `json:"access_token"`
		Proxy       *ProxyConfig `json:"proxy,omitempty"`
	}

	var token TokenWithProxy
	err := json.Unmarshal([]byte(oldTokenJSON), &token)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Proxy should be nil when not present in JSON
	if token.Proxy != nil {
		t.Errorf("Expected proxy to be nil when not in JSON, got %v", token.Proxy)
	}
}

func TestBackwardCompatibilityExistingProxy(t *testing.T) {
	// Simulate an old token structure with proxy fields
	oldTokenJSON := `{"id":"token123","access_token":"abc123","proxy":{"type":"http","host":"proxy.example.com","port":8080,"enabled":true}}`

	type TokenWithProxy struct {
		ID          string       `json:"id"`
		AccessToken string       `json:"access_token"`
		Proxy       *ProxyConfig `json:"proxy,omitempty"`
	}

	var token TokenWithProxy
	err := json.Unmarshal([]byte(oldTokenJSON), &token)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Proxy should be populated when present in JSON
	if token.Proxy == nil {
		t.Error("Expected proxy to be non-nil when in JSON")
		return
	}

	if token.Proxy.Type != ProxyTypeHTTP {
		t.Errorf("Expected proxy type to be http, got %s", token.Proxy.Type)
	}
	if token.Proxy.Host != "proxy.example.com" {
		t.Errorf("Expected proxy host to be proxy.example.com, got %s", token.Proxy.Host)
	}
	if token.Proxy.Port != 8080 {
		t.Errorf("Expected proxy port to be 8080, got %d", token.Proxy.Port)
	}
}

func TestPortBoundaryValues(t *testing.T) {
	tests := []struct {
		name    string
		pc      *ProxyConfig
		wantErr bool
	}{
		{
			name: "Minimum valid port (1)",
			pc: &ProxyConfig{
				Type:    ProxyTypeHTTP,
				Host:    "proxy.example.com",
				Port:    1,
				Enabled: true,
			},
			wantErr: false,
		},
		{
			name: "Maximum valid port (65535)",
			pc: &ProxyConfig{
				Type:    ProxyTypeHTTP,
				Host:    "proxy.example.com",
				Port:    65535,
				Enabled: true,
			},
			wantErr: false,
		},
		{
			name: "Port zero is invalid",
			pc: &ProxyConfig{
				Type:    ProxyTypeHTTP,
				Host:    "proxy.example.com",
				Port:    0,
				Enabled: true,
			},
			wantErr: true,
		},
		{
			name: "Port 65536 is invalid",
			pc: &ProxyConfig{
				Type:    ProxyTypeHTTP,
				Host:    "proxy.example.com",
				Port:    65536,
				Enabled: true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pc.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ProxyConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSpecialCharactersInHost(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		wantErr bool
	}{
		{
			name:    "Host with hyphen",
			host:    "my-proxy.example.com",
			wantErr: false,
		},
		{
			name:    "Host with underscore",
			host:    "proxy_server.example.com",
			wantErr: false,
		},
		{
			name:    "Host with numbers",
			host:    "proxy123.example.com",
			wantErr: false,
		},
		{
			name:    "IPv4 address",
			host:    "192.168.1.1",
			wantErr: false,
		},
		{
			name:    "IPv6 address",
			host:    "[2001:db8::1]",
			wantErr: false,
		},
		{
			name:    "Localhost",
			host:    "localhost",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := &ProxyConfig{
				Type:    ProxyTypeHTTP,
				Host:    tt.host,
				Port:    8080,
				Enabled: true,
			}
			err := pc.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ProxyConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestUnicodeCharactersInCredentials(t *testing.T) {
	tests := []struct {
		name    string
		pc      *ProxyConfig
		wantErr bool
	}{
		{
			name: "Username with Unicode characters",
			pc: &ProxyConfig{
				Type:     ProxyTypeHTTP,
				Host:     "proxy.example.com",
				Port:     8080,
				Username: "用户名",
				Password: "password",
				Enabled:  true,
			},
			wantErr: false,
		},
		{
			name: "Password with Unicode characters",
			pc: &ProxyConfig{
				Type:     ProxyTypeHTTP,
				Host:     "proxy.example.com",
				Port:     8080,
				Username: "user",
				Password: "пароль",
				Enabled:  true,
			},
			wantErr: false,
		},
		{
			name: "Username and password with emoji",
			pc: &ProxyConfig{
				Type:     ProxyTypeHTTP,
				Host:     "proxy.example.com",
				Port:     8080,
				Username: "user🔐",
				Password: "pass🔑",
				Enabled:  true,
			},
			wantErr: false,
		},
		{
			name: "Username with special characters",
			pc: &ProxyConfig{
				Type:     ProxyTypeHTTP,
				Host:     "proxy.example.com",
				Port:     8080,
				Username: "user@domain.com",
				Password: "p@ssw0rd!",
				Enabled:  true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pc.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ProxyConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
