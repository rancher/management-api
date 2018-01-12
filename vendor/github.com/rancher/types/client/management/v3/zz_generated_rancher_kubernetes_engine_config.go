package client

const (
	RancherKubernetesEngineConfigType                      = "rancherKubernetesEngineConfig"
	RancherKubernetesEngineConfigFieldAddons               = "addons"
	RancherKubernetesEngineConfigFieldAuthentication       = "authentication"
	RancherKubernetesEngineConfigFieldAuthorization        = "authorization"
	RancherKubernetesEngineConfigFieldEnforceDockerVersion = "enforceDockerVersion"
	RancherKubernetesEngineConfigFieldNetwork              = "network"
	RancherKubernetesEngineConfigFieldNodes                = "nodes"
	RancherKubernetesEngineConfigFieldSSHKeyPath           = "sshKeyPath"
	RancherKubernetesEngineConfigFieldServices             = "services"
	RancherKubernetesEngineConfigFieldSystemImages         = "systemImages"
)

type RancherKubernetesEngineConfig struct {
	Addons               string             `json:"addons,omitempty"`
	Authentication       *AuthnConfig       `json:"authentication,omitempty"`
	Authorization        *AuthzConfig       `json:"authorization,omitempty"`
	EnforceDockerVersion *bool              `json:"enforceDockerVersion,omitempty"`
	Network              *NetworkConfig     `json:"network,omitempty"`
	Nodes                []RKEConfigNode    `json:"nodes,omitempty"`
	SSHKeyPath           string             `json:"sshKeyPath,omitempty"`
	Services             *RKEConfigServices `json:"services,omitempty"`
	SystemImages         map[string]string  `json:"systemImages,omitempty"`
}
