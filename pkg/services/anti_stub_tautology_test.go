package services_test

import (
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
)

func TestAntiStubValidator_TautologyAndMockDetections(t *testing.T) {
	v := services.NewAntiStubValidator()

	t.Run("Python tautological assertions and self-comparisons", func(t *testing.T) {
		tests := []struct {
			name string
			code string
			rule string
		}{
			{
				name: "assert 1 == 1",
				code: "def test_foo():\n    assert 1 == 1\n",
				rule: "tautological_test_assertion",
			},
			{
				name: "assert string literal equals itself",
				code: "def test_foo():\n    assert 'test' == 'test'\n",
				rule: "tautological_test_assertion",
			},
			{
				name: "assert not False",
				code: "def test_foo():\n    assert not False\n",
				rule: "tautological_test_assertion",
			},
			{
				name: "unittest self.assertTrue(True)",
				code: "class TestFoo(unittest.TestCase):\n    def test_foo(self):\n        self.assertTrue(True)\n",
				rule: "tautological_test_assertion",
			},
			{
				name: "unittest self.assertEqual(1, 1)",
				code: "class TestFoo(unittest.TestCase):\n    def test_foo(self):\n        self.assertEqual(1, 1)\n",
				rule: "tautological_test_assertion",
			},
			{
				name: "self-comparison assert x == x",
				code: "def test_foo():\n    x = compute()\n    assert x == x\n",
				rule: "tautological_test_assertion",
			},
			{
				name: "self-comparison self.assertEqual(res, res)",
				code: "class TestFoo(unittest.TestCase):\n    def test_foo(self):\n        self.assertEqual(res, res)\n",
				rule: "tautological_test_assertion",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				violations := v.ValidateContent("tests/test_arithmetic.py", tc.code)
				assert.NotEmpty(t, violations)
				assert.Equal(t, tc.rule, violations[0].Rule)
			})
		}
	})

	t.Run("Go tautological assertions and self-comparisons", func(t *testing.T) {
		tests := []struct {
			name string
			code string
			rule string
		}{
			{
				name: "assert.True(t, true)",
				code: "func TestX(t *testing.T) {\n    assert.True(t, true)\n}\n",
				rule: "tautological_test_assertion",
			},
			{
				name: "assert.Equal(t, 1, 1)",
				code: "func TestX(t *testing.T) {\n    assert.Equal(t, 1, 1)\n}\n",
				rule: "tautological_test_assertion",
			},
			{
				name: "require.Equal(t, true, true)",
				code: "func TestX(t *testing.T) {\n    require.Equal(t, true, true)\n}\n",
				rule: "tautological_test_assertion",
			},
			{
				name: "assert.Equal(t, val, val) self-comparison",
				code: "func TestX(t *testing.T) {\n    val := compute()\n    assert.Equal(t, val, val)\n}\n",
				rule: "tautological_test_assertion",
			},
			{
				name: "require.Equal(t, out, out) self-comparison",
				code: "func TestX(t *testing.T) {\n    out := run()\n    require.Equal(t, out, out)\n}\n",
				rule: "tautological_test_assertion",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				violations := v.ValidateContent("pkg/foo/foo_test.go", tc.code)
				assert.NotEmpty(t, violations)
				assert.Equal(t, tc.rule, violations[0].Rule)
			})
		}
	})

	t.Run("C/C++ and Rust tautological assertions", func(t *testing.T) {
		tests := []struct {
			name string
			file string
			code string
			rule string
		}{
			{
				name: "C assert(1)",
				file: "tests/unit/test_store.c",
				code: "void test_store(void) {\n    assert(1);\n}\n",
				rule: "tautological_test_assertion",
			},
			{
				name: "C assert(1 == 1)",
				file: "tests/unit/test_store.c",
				code: "void test_store(void) {\n    assert(1 == 1);\n}\n",
				rule: "tautological_test_assertion",
			},
			{
				name: "C assert(x == x) self-comparison",
				file: "tests/unit/test_store.c",
				code: "void test_store(void) {\n    int x = 5;\n    assert(x == x);\n}\n",
				rule: "tautological_test_assertion",
			},
			{
				name: "C EXPECT_EQ(val, val) self-comparison",
				file: "tests/unit/test_store.cpp",
				code: "void test_store(void) {\n    EXPECT_EQ(val, val);\n}\n",
				rule: "tautological_test_assertion",
			},
			{
				name: "Rust assert!(true)",
				file: "tests/test_store.rs",
				code: "#[test]\nfn test_store() {\n    assert!(true);\n}\n",
				rule: "tautological_test_assertion",
			},
			{
				name: "Rust assert_eq!(1, 1)",
				file: "tests/test_store.rs",
				code: "#[test]\nfn test_store() {\n    assert_eq!(1, 1);\n}\n",
				rule: "tautological_test_assertion",
			},
			{
				name: "Rust assert_eq!(res, res) self-comparison",
				file: "tests/test_store.rs",
				code: "#[test]\nfn test_store() {\n    let res = 5;\n    assert_eq!(res, res);\n}\n",
				rule: "tautological_test_assertion",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				violations := v.ValidateContent(tc.file, tc.code)
				assert.NotEmpty(t, violations)
				assert.Contains(t, []string{tc.rule, "python_empty_stub_function"}, violations[0].Rule)
			})
		}
	})

	t.Run("JavaScript and TypeScript tautological assertions", func(t *testing.T) {
		tests := []struct {
			name string
			code string
			rule string
		}{
			{
				name: "expect(true).toBe(true)",
				code: "test('something', () => {\n    expect(true).toBe(true);\n});\n",
				rule: "tautological_test_assertion",
			},
			{
				name: "expect(1).toBe(1)",
				code: "it('something', () => {\n    expect(1).toBe(1);\n});\n",
				rule: "tautological_test_assertion",
			},
			{
				name: "expect(res).toBe(res) self-comparison",
				code: "test('something', () => {\n    const res = run();\n    expect(res).toBe(res);\n});\n",
				rule: "tautological_test_assertion",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				violations := v.ValidateContent("tests/app.spec.ts", tc.code)
				assert.NotEmpty(t, violations)
				assert.Equal(t, tc.rule, violations[0].Rule)
			})
		}
	})

	t.Run("Vacuous and empty test functions", func(t *testing.T) {
		tests := []struct {
			name string
			file string
			code string
			rule string
		}{
			{
				name: "Python def test with only pass",
				file: "tests/test_app.py",
				code: "def test_empty():\n    pass\n",
				rule: "vacuous_empty_test",
			},
			{
				name: "Python def test with docstring and ellipsis",
				file: "tests/test_app.py",
				code: "def test_empty():\n    \"\"\"This does nothing\"\"\"\n    ...\n",
				rule: "vacuous_empty_test",
			},
			{
				name: "Python def test with only return",
				file: "tests/test_app.py",
				code: "def test_empty():\n    return\n",
				rule: "vacuous_empty_test",
			},
			{
				name: "Go inline empty test",
				file: "pkg/api/handler_test.go",
				code: "func TestHandler(t *testing.T) {}\n",
				rule: "vacuous_empty_test",
			},
			{
				name: "Go multi-line empty test",
				file: "pkg/api/handler_test.go",
				code: "func TestHandler(t *testing.T) {\n    // TODO implement\n}\n",
				rule: "vacuous_empty_test",
			},
			{
				name: "C inline empty test",
				file: "tests/test_http.c",
				code: "void test_http(void) {}\n",
				rule: "vacuous_empty_test",
			},
			{
				name: "C empty test with return 0",
				file: "tests/test_http.c",
				code: "int test_http(void) {\n    return 0;\n}\n",
				rule: "vacuous_empty_test",
			},
			{
				name: "JS inline empty test",
				file: "tests/unit.test.js",
				code: "it('does nothing', () => {});\n",
				rule: "vacuous_empty_test",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				violations := v.ValidateContent(tc.file, tc.code)
				assert.NotEmpty(t, violations)
				assert.Contains(t, []string{tc.rule, "python_empty_stub_function"}, violations[0].Rule)
			})
		}
	})

	t.Run("Evasive test skipping", func(t *testing.T) {
		tests := []struct {
			name string
			file string
			code string
			rule string
		}{
			{
				name: "pytest.mark.skip",
				file: "tests/test_feature.py",
				code: "@pytest.mark.skip(reason='broken')\ndef test_feature():\n    assert 1 == 2\n",
				rule: "evasive_test_skipping",
			},
			{
				name: "unittest.skip",
				file: "tests/test_feature.py",
				code: "@unittest.skip\ndef test_feature():\n    assert 1 == 2\n",
				rule: "evasive_test_skipping",
			},
			{
				name: "Rust #[ignore]",
				file: "tests/test_feature.rs",
				code: "#[test]\n#[ignore]\nfn test_feature() {\n    assert_eq!(1, 2);\n}\n",
				rule: "evasive_test_skipping",
			},
			{
				name: "JS test.skip",
				file: "tests/feature.spec.ts",
				code: "test.skip('feature', () => {\n    expect(1).toBe(2);\n});\n",
				rule: "evasive_test_skipping",
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				violations := v.ValidateContent(tc.file, tc.code)
				assert.NotEmpty(t, violations)
				assert.Contains(t, []string{tc.rule, "python_empty_stub_function"}, violations[0].Rule)
			})
		}
	})

	t.Run("Valid real tests must pass with zero violations", func(t *testing.T) {
		validTests := []struct {
			name string
			file string
			code string
		}{
			{
				name: "Real Python test",
				file: "tests/test_math.py",
				code: `
def test_addition():
    result = add(2, 3)
    assert result == 5
`,
			},
			{
				name: "Real Go test",
				file: "pkg/calc/calc_test.go",
				code: `
func TestAdd(t *testing.T) {
    got := Add(2, 3)
    assert.Equal(t, 5, got)
}
`,
			},
			{
				name: "Real C test",
				file: "tests/test_store.c",
				code: `
void test_put_get(void) {
    int rc = store_put("key", "val");
    assert(rc == 0);
}
`,
			},
			{
				name: "Real Rust test",
				file: "tests/test_calc.rs",
				code: `
#[test]
fn test_add() {
    let result = add(2, 3);
    assert_eq!(result, 5);
}
`,
			},
			{
				name: "Real JS test",
				file: "tests/calc.test.ts",
				code: `
test('adds numbers', () => {
    const sum = add(2, 3);
    expect(sum).toBe(5);
});
`,
			},
		}

		for _, tc := range validTests {
			t.Run(tc.name, func(t *testing.T) {
				violations := v.ValidateContent(tc.file, tc.code)
				assert.Empty(t, violations)
			})
		}
	})
}
