package codec

import "testing"

// TestFlexDecimal_NumberAndStringForms pins that FlexDecimal decodes both the
// bare-number and quoted-string wire forms Gate uses interchangeably, preserves
// precision, and degrades gracefully (Zero) on null/empty/malformed tokens.
func TestFlexDecimal_NumberAndStringForms(t *testing.T) {
	var cases = []struct {
		in   string
		want string
	}{
		{`25`, "25"},
		{`"25"`, "25"},
		{`0.0418`, "0.0418"},
		{`"0.0418"`, "0.0418"},
		{`-0.7`, "-0.7"},
		{`"-0.7"`, "-0.7"},
		{`""`, "0"},
		{`null`, "0"},
		{`"abc"`, "0"}, // malformed: tolerant Zero, no error
	}
	var i int
	for i = 0; i < len(cases); i++ {
		var v FlexDecimal
		var err error
		err = v.UnmarshalJSON([]byte(cases[i].in))
		if err != nil {
			t.Fatalf("in=%s: unexpected error %v", cases[i].in, err)
		}
		if v.String() != cases[i].want {
			t.Fatalf("in=%s: got %s want %s", cases[i].in, v.String(), cases[i].want)
		}
	}
}
