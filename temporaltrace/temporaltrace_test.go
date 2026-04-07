package temporaltrace

import "testing"

func TestNewInterceptor_defaults(t *testing.T) {
	i, err := NewInterceptor()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if i == nil {
		t.Fatal("expected non-nil interceptor")
	}
}

func TestNewInterceptor_withoutSignalTracing(t *testing.T) {
	i, err := NewInterceptor(WithoutSignalTracing())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if i == nil {
		t.Fatal("expected non-nil interceptor")
	}
}

func TestNewInterceptor_withoutQueryTracing(t *testing.T) {
	i, err := NewInterceptor(WithoutQueryTracing())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if i == nil {
		t.Fatal("expected non-nil interceptor")
	}
}

func TestOptions_defaults(t *testing.T) {
	var cfg options
	if cfg.disableSignalTracing {
		t.Fatal("expected disableSignalTracing to default to false")
	}
	if cfg.disableQueryTracing {
		t.Fatal("expected disableQueryTracing to default to false")
	}
}
