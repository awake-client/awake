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
func (service *Service) SecretStates(workspaceRoot string, scope string, environmentFileName string) ([]SecretState, error) {
	return service.repository.States(workspaceRoot, scope, environmentFileName)
}

// SetSecret stores or replaces the value of a declared secret.
func (service *Service) SetSecret(workspaceRoot string, scope string, environmentFileName string, name string, value string) error {
	return service.repository.Set(workspaceRoot, scope, environmentFileName, name, value)
}

// DeleteSecret removes a declared secret from the keychain.
func (service *Service) DeleteSecret(workspaceRoot string, scope string, environmentFileName string, name string) error {
	return service.repository.Delete(workspaceRoot, scope, environmentFileName, name)
}

// CopySecrets copies the stored secret values of an environment to another
// one of the same scope, such as a clone. Values never cross the bridge.
func (service *Service) CopySecrets(workspaceRoot string, scope string, sourceFileName string, targetFileName string) error {
	return service.repository.Copy(workspaceRoot, scope, sourceFileName, targetFileName)
}

// MoveSecrets transfers the stored secret values of an environment renamed
// from formerFileName to fileName.
func (service *Service) MoveSecrets(workspaceRoot string, scope string, formerFileName string, fileName string) error {
	return service.repository.Move(workspaceRoot, scope, formerFileName, fileName)
}
