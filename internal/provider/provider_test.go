// Package provider tests the Authenticator interface and implementations
package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auth "github.com/sunbankio/qwencoder-proxy/internal/token"
)

// MockAuthenticator is a mock implementation for testing
type MockAuthenticator struct {
	token         string
	authenticated bool
	credsPath     string
	clearCalled   bool
}

func (m *MockAuthenticator) Authenticate(ctx context.Context) error {
	m.authenticated = true
	return nil
}

func (m *MockAuthenticator) GetToken(ctx context.Context) (string, error) {
	return m.token, nil
}

func (m *MockAuthenticator) GetTokenWithClient(ctx context.Context) (string, *http.Client, error) {
	token, err := m.GetToken(ctx)
	return token, nil, err
}

func (m *MockAuthenticator) GetHTTPClient() (*http.Client, error) {
	return nil, errors.New("GetHTTPClient not implemented for MockAuthenticator")
}

func (m *MockAuthenticator) IsAuthenticated() bool {
	return m.authenticated
}

func (m *MockAuthenticator) GetCredentialsPath() string {
	return m.credsPath
}

func (m *MockAuthenticator) ClearCredentials() error {
	m.clearCalled = true
	return nil
}

// TestAuthenticatorInterfaceBackwardCompatibility tests that existing methods still work
func TestAuthenticatorInterfaceBackwardCompatibility(t *testing.T) {
	mock := &MockAuthenticator{
		token:         "test-token",
		authenticated: false,
		credsPath:     "/test/path",
	}

	ctx := context.Background()

	// Test Authenticate
	err := mock.Authenticate(ctx)
	require.NoError(t, err)
	assert.True(t, mock.IsAuthenticated())

	// Test GetToken
	token, err := mock.GetToken(ctx)
	require.NoError(t, err)
	assert.Equal(t, "test-token", token)

	// Test IsAuthenticated
	assert.True(t, mock.IsAuthenticated())

	// Test GetCredentialsPath
	assert.Equal(t, "/test/path", mock.GetCredentialsPath())

	// Test ClearCredentials
	err = mock.ClearCredentials()
	require.NoError(t, err)
	assert.True(t, mock.clearCalled)
}

// TestGetTokenWithClientDefaultImplementation tests the default GetTokenWithClient implementation
func TestGetTokenWithClientDefaultImplementation(t *testing.T) {
	mock := &MockAuthenticator{
		token:         "test-token",
		authenticated: true,
	}

	ctx := context.Background()

	// Test GetTokenWithClient - should return token and nil client
	token, client, err := mock.GetTokenWithClient(ctx)
	require.NoError(t, err)
	assert.Equal(t, "test-token", token)
	assert.Nil(t, client, "Default implementation should return nil for client")
}

// TestGetHTTPClientDefaultImplementation tests the default GetHTTPClient implementation
func TestGetHTTPClientDefaultImplementation(t *testing.T) {
	mock := &MockAuthenticator{}

	// Test GetHTTPClient - should return error
	client, err := mock.GetHTTPClient()
	assert.Error(t, err, "GetHTTPClient should return an error")
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "not implemented", "Error message should indicate not implemented")
}

// TestAuthenticatorInterfaceCompliance tests that mock implements all interface methods
func TestAuthenticatorInterfaceCompliance(t *testing.T) {
	var _ Authenticator = &MockAuthenticator{}
	// This will fail to compile if MockAuthenticator doesn't implement all methods
	_ = struct{}{} // Silence unused variable
}

// MockTokenManagerAware is a mock provider that implements TokenManagerAware
type MockTokenManagerAware struct {
	providerType  ProviderType
	tokenManager  *auth.TokenManager
	authenticator Authenticator
}

func (m *MockTokenManagerAware) Name() ProviderType {
	return m.providerType
}

func (m *MockTokenManagerAware) Protocol() ProtocolType {
	return ProtocolOpenAI
}

func (m *MockTokenManagerAware) SupportedModels() []string {
	return []string{"test-model"}
}

func (m *MockTokenManagerAware) SupportsModel(model string) bool {
	return model == "test-model"
}

func (m *MockTokenManagerAware) GenerateContent(ctx context.Context, model string, request interface{}) (interface{}, error) {
	return nil, nil
}

func (m *MockTokenManagerAware) GenerateContentStream(ctx context.Context, model string, request interface{}) (io.ReadCloser, error) {
	return nil, nil
}

func (m *MockTokenManagerAware) ListModels(ctx context.Context) (interface{}, error) {
	return nil, nil
}

func (m *MockTokenManagerAware) GetAuthenticator() Authenticator {
	return m.authenticator
}

func (m *MockTokenManagerAware) IsHealthy(ctx context.Context) bool {
	return true
}

func (m *MockTokenManagerAware) SetTokenManager(manager *auth.TokenManager) {
	m.tokenManager = manager
}

// MockNonTokenManagerAware is a mock provider that does NOT implement TokenManagerAware
type MockNonTokenManagerAware struct {
	providerType  ProviderType
	authenticator Authenticator
}

func (m *MockNonTokenManagerAware) Name() ProviderType {
	return m.providerType
}

func (m *MockNonTokenManagerAware) Protocol() ProtocolType {
	return ProtocolOpenAI
}

func (m *MockNonTokenManagerAware) SupportedModels() []string {
	return []string{"test-model"}
}

func (m *MockNonTokenManagerAware) SupportsModel(model string) bool {
	return model == "test-model"
}

func (m *MockNonTokenManagerAware) GenerateContent(ctx context.Context, model string, request interface{}) (interface{}, error) {
	return nil, nil
}

func (m *MockNonTokenManagerAware) GenerateContentStream(ctx context.Context, model string, request interface{}) (io.ReadCloser, error) {
	return nil, nil
}

func (m *MockNonTokenManagerAware) ListModels(ctx context.Context) (interface{}, error) {
	return nil, nil
}

func (m *MockNonTokenManagerAware) GetAuthenticator() Authenticator {
	return m.authenticator
}

func (m *MockNonTokenManagerAware) IsHealthy(ctx context.Context) bool {
	return true
}

// TestTokenManagerAwareInterface tests that providers can implement TokenManagerAware
func TestTokenManagerAwareInterface(t *testing.T) {
	mock := &MockTokenManagerAware{
		providerType:  ProviderQwen,
		authenticator: &MockAuthenticator{},
	}

	// Verify it implements Provider interface
	var _ Provider = mock

	// Verify it implements TokenManagerAware
	var _ TokenManagerAware = mock

	assert.Nil(t, mock.tokenManager, "TokenManager should be nil initially")

	// Create a mock TokenManager
	tokenManager := &auth.TokenManager{}

	// Set the token manager
	mock.SetTokenManager(tokenManager)

	assert.NotNil(t, mock.tokenManager, "TokenManager should be set")
	assert.Equal(t, tokenManager, mock.tokenManager, "TokenManager should match")
}

// TestNonTokenManagerAwareProvider tests that providers without TokenManagerAware still work
func TestNonTokenManagerAwareProvider(t *testing.T) {
	mock := &MockNonTokenManagerAware{
		providerType:  ProviderQwen,
		authenticator: &MockAuthenticator{},
	}

	// Verify it implements Provider interface
	var _ Provider = mock

	// Verify it does NOT implement TokenManagerAware
	_, ok := interface{}(mock).(TokenManagerAware)
	assert.False(t, ok, "Provider should not implement TokenManagerAware")
}

// TestFactoryRegisterWithTokenManager tests the RegisterWithTokenManager method
func TestFactoryRegisterWithTokenManager(t *testing.T) {
	factory := NewFactory()

	// Create a mock TokenManager
	tokenManager := &auth.TokenManager{}

	// Create a provider that implements TokenManagerAware
	awareProvider := &MockTokenManagerAware{
		providerType:  ProviderQwen,
		authenticator: &MockAuthenticator{},
	}

	// Register with token manager
	factory.RegisterWithTokenManager(awareProvider, tokenManager)

	// Verify the provider was registered
	retrieved, err := factory.Get(ProviderQwen)
	require.NoError(t, err)
	assert.Equal(t, awareProvider, retrieved)

	// Verify the token manager was injected
	assert.Equal(t, tokenManager, awareProvider.tokenManager)
}

// TestFactoryRegisterWithTokenManagerNonAware tests RegisterWithTokenManager with non-aware provider
func TestFactoryRegisterWithTokenManagerNonAware(t *testing.T) {
	factory := NewFactory()

	// Create a mock TokenManager
	tokenManager := &auth.TokenManager{}

	// Create a provider that does NOT implement TokenManagerAware
	nonAwareProvider := &MockNonTokenManagerAware{
		providerType:  ProviderGeminiCLI,
		authenticator: &MockAuthenticator{},
	}

	// Register with token manager - should not panic
	factory.RegisterWithTokenManager(nonAwareProvider, tokenManager)

	// Verify the provider was still registered
	retrieved, err := factory.Get(ProviderGeminiCLI)
	require.NoError(t, err)
	assert.Equal(t, nonAwareProvider, retrieved)
}

// TestFactoryRegisterBackwardCompatibility tests that Register still works without TokenManager
func TestFactoryRegisterBackwardCompatibility(t *testing.T) {
	factory := NewFactory()

	// Create a provider that implements TokenManagerAware
	awareProvider := &MockTokenManagerAware{
		providerType:  ProviderQwen,
		authenticator: &MockAuthenticator{},
	}

	// Register without token manager - backward compatibility
	factory.Register(awareProvider)

	// Verify the provider was registered
	retrieved, err := factory.Get(ProviderQwen)
	require.NoError(t, err)
	assert.Equal(t, awareProvider, retrieved)

	// Token manager should not be set
	assert.Nil(t, awareProvider.tokenManager)
}
