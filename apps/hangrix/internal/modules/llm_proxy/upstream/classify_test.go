package upstream

import "testing"

func TestClassifyDispatchError_FailsOverOnProviderAuthFailures(t *testing.T) {
	t.Parallel()

	for _, code := range []int{401, 403} {
		cls := ClassifyDispatchError(&UpstreamError{StatusCode: code, Message: "provider auth failure"})
		if !cls.FailOver {
			t.Fatalf("status %d: FailOver=false, want true", code)
		}
		if cls.StatusCode != code {
			t.Fatalf("status %d: StatusCode=%d, want %d", code, cls.StatusCode, code)
		}
	}
}

func TestClassifyDispatchError_StopsOnRequestShapeErrors(t *testing.T) {
	t.Parallel()

	for _, code := range []int{400, 404, 422} {
		cls := ClassifyDispatchError(&UpstreamError{StatusCode: code, Message: "request problem"})
		if cls.FailOver {
			t.Fatalf("status %d: FailOver=true, want false", code)
		}
	}
}
