package services_test

import (
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
)

func TestAntiStubValidator_MockCallVerification(t *testing.T) {
	v := services.NewAntiStubValidator()

	t.Run("Python mock injection requires call assertion", func(t *testing.T) {
		t.Run("when python creates Mock() but never asserts it was called, it flags violation", func(t *testing.T) {
			code := `
def test_order_processor():
    mock_mailer = Mock()
    processor = OrderProcessor(mailer=mock_mailer)
    res = processor.process(123)
    assert res == True
`
			violations := v.ValidateContent("tests/test_orders.py", code)
			assert.NotEmpty(t, violations)
			assert.Equal(t, "unverified_mock_assertion", violations[0].Rule)
			assert.Contains(t, violations[0].Snippet, "test instantiates/injects mock dependency")
		})

		t.Run("when python uses @patch but never asserts mock calls, it flags violation", func(t *testing.T) {
			code := `
@patch("app.services.send_email")
def test_signup(mock_send):
    user = signup("alice@example.com")
    assert user.is_active is True
`
			violations := v.ValidateContent("tests/test_signup.py", code)
			assert.NotEmpty(t, violations)
			assert.Equal(t, "unverified_mock_assertion", violations[0].Rule)
		})

		t.Run("when python asserts mock call with assert_called_once_with, it passes", func(t *testing.T) {
			code := `
def test_order_processor():
    mock_mailer = Mock()
    processor = OrderProcessor(mailer=mock_mailer)
    res = processor.process(123)
    mock_mailer.send.assert_called_once_with(123)
    assert res == True
`
			violations := v.ValidateContent("tests/test_orders.py", code)
			assert.Empty(t, violations)
		})

		t.Run("when python asserts mock call with assert_not_called, it passes", func(t *testing.T) {
			code := `
def test_order_processor_invalid():
    mock_mailer = Mock()
    processor = OrderProcessor(mailer=mock_mailer)
    res = processor.process(-1)
    mock_mailer.send.assert_not_called()
    assert res == False
`
			violations := v.ValidateContent("tests/test_orders.py", code)
			assert.Empty(t, violations)
		})
	})

	t.Run("Go mock dependency injection requires call assertion", func(t *testing.T) {
		t.Run("when Go sets expectations with .On() but never calls AssertExpectations, it flags violation", func(t *testing.T) {
			code := `
func TestOrderService(t *testing.T) {
    mockStore := new(MockStore)
    mockStore.On("Save", mock.Anything).Return(nil)
    svc := NewOrderService(mockStore)
    err := svc.PlaceOrder(123)
    assert.NoError(t, err)
}
`
			violations := v.ValidateContent("pkg/orders/service_test.go", code)
			assert.NotEmpty(t, violations)
			assert.Equal(t, "unverified_mock_assertion", violations[0].Rule)
			assert.Contains(t, violations[0].Snippet, "test sets up mock expectations")
		})

		t.Run("when Go calls mockStore.AssertExpectations(t), it passes", func(t *testing.T) {
			code := `
func TestOrderService(t *testing.T) {
    mockStore := new(MockStore)
    mockStore.On("Save", mock.Anything).Return(nil)
    svc := NewOrderService(mockStore)
    err := svc.PlaceOrder(123)
    assert.NoError(t, err)
    mockStore.AssertExpectations(t)
}
`
			violations := v.ValidateContent("pkg/orders/service_test.go", code)
			assert.Empty(t, violations)
		})

		t.Run("when Go asserts mock call with AssertCalled, it passes", func(t *testing.T) {
			code := `
func TestOrderService(t *testing.T) {
    mockStore := new(MockStore)
    mockStore.On("Save", mock.Anything).Return(nil)
    svc := NewOrderService(mockStore)
    _ = svc.PlaceOrder(123)
    mockStore.AssertCalled(t, "Save", 123)
}
`
			violations := v.ValidateContent("pkg/orders/service_test.go", code)
			assert.Empty(t, violations)
		})
	})

	t.Run("JavaScript / TypeScript mock verification", func(t *testing.T) {
		t.Run("when jest.fn() is created but never checked with toHaveBeenCalled, it flags violation", func(t *testing.T) {
			code := `
test("dispatches event", () => {
    const mockHandler = jest.fn();
    const emitter = new EventEmitter();
    emitter.on("click", mockHandler);
    emitter.emit("click");
});
`
			violations := v.ValidateContent("tests/emitter.spec.ts", code)
			assert.NotEmpty(t, violations)
			assert.Equal(t, "unverified_mock_assertion", violations[0].Rule)
			assert.Contains(t, violations[0].Snippet, "test creates mock function but never asserts that it was called")
		})

		t.Run("when expect(mockHandler).toHaveBeenCalledWith is present, it passes", func(t *testing.T) {
			code := `
test("dispatches event", () => {
    const mockHandler = jest.fn();
    const emitter = new EventEmitter();
    emitter.on("click", mockHandler);
    emitter.emit("click", "data");
    expect(mockHandler).toHaveBeenCalledWith("data");
});
`
			violations := v.ValidateContent("tests/emitter.spec.ts", code)
			assert.Empty(t, violations)
		})
	})
}
