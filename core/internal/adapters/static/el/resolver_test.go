package el

import (
	"testing"

	"legacylens/core/internal/domain"
)

func TestResolveManagedBeanAndRemoteCommand(t *testing.T) {
	symbols := []domain.Symbol{{ID: "method", QualifiedName: "com.example.OrdersBean.save", Kind: "method"}}
	relations, diagnostics := NewResolver().Resolve("#{orders.save}", symbols)
	if len(diagnostics) != 0 || len(relations) != 1 || relations[0].ToID == nil || *relations[0].ToID != "method" || relations[0].Resolution != domain.ResolutionResolved {
		t.Fatalf("EL resolution = %#v, diagnostics = %#v", relations, diagnostics)
	}
}
