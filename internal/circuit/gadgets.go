package circuit

import (
	"github.com/consensys/gnark/frontend"
)

// Gadgets contains the reusable integer and conditional constraint helpers.
// Every arithmetic helper constrains its operands and result to the documented
// unsigned width, so native-field wraparound cannot stand in for integer
// overflow or underflow.
type Gadgets struct {
	api frontend.API
}

// NewGadgets constructs integer gadgets for one circuit API.
func NewGadgets(api frontend.API) Gadgets {
	return Gadgets{api: api}
}

// AssertUnsigned constrains value to [0, 2^width).
func (g Gadgets) AssertUnsigned(value frontend.Variable, width int) {
	if width < 1 || width > 253 {
		panic("unsupported unsigned width")
	}
	g.api.ToBinary(value, width)
}

// AssertBoolean constrains a protocol flag.
func (g Gadgets) AssertBoolean(value frontend.Variable) {
	g.api.AssertIsBoolean(value)
}

// AssertEnum constrains value to one of the listed constants.
func (g Gadgets) AssertEnum(value frontend.Variable, allowed ...uint64) {
	if len(allowed) == 0 {
		panic("enum must have at least one value")
	}
	product := frontend.Variable(1)
	for _, item := range allowed {
		product = g.api.Mul(product, g.api.Sub(value, item))
	}
	g.api.AssertIsEqual(product, 0)
}

// IsEqual returns a fully constrained equality flag.
func (g Gadgets) IsEqual(left, right frontend.Variable) frontend.Variable {
	return g.api.IsZero(g.api.Sub(left, right))
}

// IsLess compares two unsigned values of the given common width.
func (g Gadgets) IsLess(left, right frontend.Variable, width int) frontend.Variable {
	leftBits := g.api.ToBinary(left, width)
	rightBits := g.api.ToBinary(right, width)
	less := frontend.Variable(0)
	equalPrefix := frontend.Variable(1)
	for index := width - 1; index >= 0; index-- {
		leftZeroRightOne := g.api.Mul(g.api.Sub(1, leftBits[index]), rightBits[index])
		less = g.api.Add(less, g.api.Mul(equalPrefix, leftZeroRightOne))
		bitsEqual := g.api.Sub(
			1,
			g.api.Add(
				leftBits[index],
				rightBits[index],
				g.api.Mul(-2, leftBits[index], rightBits[index]),
			),
		)
		equalPrefix = g.api.Mul(equalPrefix, bitsEqual)
	}
	g.api.AssertIsBoolean(less)
	return less
}

// IsLessOrEqual compares two unsigned values of the given common width.
func (g Gadgets) IsLessOrEqual(left, right frontend.Variable, width int) frontend.Variable {
	return g.api.Add(g.IsLess(left, right, width), g.IsEqual(left, right))
}

// Min returns the smaller of two range-constrained unsigned integers.
func (g Gadgets) Min(left, right frontend.Variable, width int) frontend.Variable {
	return g.api.Select(g.IsLess(left, right, width), left, right)
}

// Select returns whenTrue when selector is one, otherwise whenFalse.
func (g Gadgets) Select(
	selector frontend.Variable,
	whenTrue frontend.Variable,
	whenFalse frontend.Variable,
) frontend.Variable {
	g.api.AssertIsBoolean(selector)
	return g.api.Select(selector, whenTrue, whenFalse)
}

// AssertEqualWhen gates an equality constraint with a boolean condition.
func (g Gadgets) AssertEqualWhen(
	condition frontend.Variable,
	left frontend.Variable,
	right frontend.Variable,
) {
	g.api.AssertIsBoolean(condition)
	g.api.AssertIsEqual(g.api.Mul(condition, g.api.Sub(left, right)), 0)
}

// AddNoOverflow performs an unsigned addition that must fit width bits.
func (g Gadgets) AddNoOverflow(
	left frontend.Variable,
	right frontend.Variable,
	width int,
) frontend.Variable {
	g.AssertUnsigned(left, width)
	g.AssertUnsigned(right, width)
	result := g.api.Add(left, right)
	g.AssertUnsigned(result, width)
	return result
}

// SubNoUnderflow performs an unsigned subtraction with left >= right.
func (g Gadgets) SubNoUnderflow(
	left frontend.Variable,
	right frontend.Variable,
	width int,
) frontend.Variable {
	g.AssertUnsigned(left, width)
	g.AssertUnsigned(right, width)
	g.api.AssertIsEqual(g.IsLess(left, right, width), 0)
	result := g.api.Sub(left, right)
	g.AssertUnsigned(result, width)
	return result
}

// MulNoOverflow performs an unsigned multiplication that must fit width bits.
// Width is limited to 64 so the unreduced product is below the BN254 modulus.
func (g Gadgets) MulNoOverflow(
	left frontend.Variable,
	right frontend.Variable,
	width int,
) frontend.Variable {
	if width > 64 {
		panic("MulNoOverflow only supports widths through 64 bits")
	}
	g.AssertUnsigned(left, width)
	g.AssertUnsigned(right, width)
	result := g.api.Mul(left, right)
	g.AssertUnsigned(result, width)
	return result
}

// Mul64To128 returns the exact 128-bit product of two uint64 values.
func (g Gadgets) Mul64To128(
	left frontend.Variable,
	right frontend.Variable,
) frontend.Variable {
	g.AssertUnsigned(left, 64)
	g.AssertUnsigned(right, 64)
	result := g.api.Mul(left, right)
	g.AssertUnsigned(result, 128)
	return result
}

// MulBounded returns an exact product with independently documented operand
// and result widths. The total operand width must stay below the field width.
func (g Gadgets) MulBounded(
	left frontend.Variable,
	leftWidth int,
	right frontend.Variable,
	rightWidth int,
	resultWidth int,
) frontend.Variable {
	if leftWidth+rightWidth > 253 {
		panic("MulBounded operands may wrap the native field")
	}
	g.AssertUnsigned(left, leftWidth)
	g.AssertUnsigned(right, rightWidth)
	result := g.api.Mul(left, right)
	g.AssertUnsigned(result, resultWidth)
	return result
}
