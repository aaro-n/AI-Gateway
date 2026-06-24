package provider

import (
	"github.com/gin-gonic/gin"
)

type ProtocolDescriptor struct {
	Name           string
	KeyPrefix      string
	KeyLength      int
	KeyEncoder     func([]byte) string
	AuthExtractor  func(c *gin.Context) string
	ModelExtractor func(c *gin.Context) (string, error)
	DefaultBaseURL string
	NewProvider    func(cfg *Config) Provider
}

var registry = make(map[string]ProtocolDescriptor)

func Register(desc ProtocolDescriptor) {
	registry[desc.Name] = desc
}

func GetProtocol(name string) (ProtocolDescriptor, bool) {
	desc, ok := registry[name]
	return desc, ok
}

func AllProtocols() map[string]ProtocolDescriptor {
	return registry
}

func ProtocolNames() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}
