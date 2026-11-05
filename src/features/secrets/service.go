package secrets

// Service exposes secret storage to the application layer. Values can be
// written and checked but never read back through this service.
type Service struct {
	repository *Repository
}

// NewService returns a secret service backed by the operating system keychain.
func NewService() *Service {
	return NewServiceWithKeychain(NewKeyringKeychain())
}

// NewServiceWithKeychain returns a secret service backed by keychain.
func NewServiceWithKeychain(keychain Keychain) *Service {
	return &Service{repository: NewRepository(keychain)}
}

// SecretStates reports which declared secrets of an environment are stored.
func (service *Service) SecretStates(workspaceRoot string, environmentFileName string) ([]SecretState, error) {
	return service.repository.States(workspaceRoot, environmentFileName)
}

// SetSecret stores or replaces the value of a declared secret.
func (service *Service) SetSecret(workspaceRoot string, environmentFileName string, name string, value string) error {
	return service.repository.Set(workspaceRoot, environmentFileName, name, value)
}

// DeleteSecret removes a declared secret from the keychain.
func (service *Service) DeleteSecret(workspaceRoot string, environmentFileName string, name string) error {
	return service.repository.Delete(workspaceRoot, environmentFileName, name)
}
