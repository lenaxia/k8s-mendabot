package k8sgpt

import (
	"github.com/lenaxia/k8s-mendabot/internal/domain"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Compile-time assertion: K8sGPTProvider satisfies domain.SourceProvider.
var _ domain.SourceProvider = (*K8sGPTProvider)(nil)

// K8sGPTProvider watches k8sgpt Result CRDs and extracts Findings from them.
type K8sGPTProvider struct{}

// ProviderName returns the stable identifier for this provider.
func (p *K8sGPTProvider) ProviderName() string { panic("not implemented") }

// ObjectType returns the runtime.Object type this provider watches.
func (p *K8sGPTProvider) ObjectType() client.Object { panic("not implemented") }

// ExtractFinding converts a watched Result object into a Finding.
func (p *K8sGPTProvider) ExtractFinding(obj client.Object) (*domain.Finding, error) {
	panic("not implemented")
}

// Fingerprint computes the deduplication key for the given Finding.
func (p *K8sGPTProvider) Fingerprint(f *domain.Finding) string { panic("not implemented") }
