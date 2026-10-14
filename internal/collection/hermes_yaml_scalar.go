package collection

import (
	"regexp"
	"strings"
	"unicode"
)

var hermesYAMLFloatSyntax = regexp.MustCompile(`^(?:\p{Nd}+(?:\.\p{Nd}*)?|\.\p{Nd}+)(?:e[+-]?\p{Nd}+)?$`)

func hermesYAMLScalarValid(tag, value string) bool {
	switch tag {
	case "!!bool":
		switch strings.ToLower(value) {
		case "yes", "no", "true", "false", "on", "off":
			return true
		}
		return false
	case "!!int", "!!float":
		value = strings.ReplaceAll(value, "_", "")
		if value == "" {
			return false
		}
		if value[0] == '+' || value[0] == '-' {
			value = value[1:]
		}
		if value == "" {
			return false
		}
		if tag == "!!int" {
			switch {
			case strings.HasPrefix(value, "0b"):
				return hermesYAMLIntegerSyntax(value[2:], 2)
			case strings.HasPrefix(value, "0x"):
				return hermesYAMLIntegerSyntax(value[2:], 16)
			case value[0] == '0':
				return hermesYAMLIntegerSyntax(value, 8)
			}
			for _, part := range strings.Split(value, ":") {
				if !hermesYAMLIntegerSyntax(part, 10) {
					return false
				}
			}
			return true
		}
		value = strings.Map(func(char rune) rune {
			if char >= 'A' && char <= 'Z' {
				return char + ('a' - 'A')
			}
			return char
		}, value)
		if value == ".inf" || value == ".nan" {
			return true
		}
		for _, part := range strings.Split(value, ":") {
			if !hermesYAMLPythonFloatSyntax(part) {
				return false
			}
		}
		return true
	case "!!binary":
		return hermesYAMLBinarySyntax(value)
	}
	return true
}

func hermesYAMLIntegerSyntax(value string, base int) bool {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
		value = value[1:]
	}
	digits := []rune(value)
	if len(digits) >= 2 && hermesYAMLDecimalDigit(digits[0]) == 0 {
		prefix := digits[1]
		if base == 2 && (prefix == 'b' || prefix == 'B') || base == 8 && (prefix == 'o' || prefix == 'O') || base == 16 && (prefix == 'x' || prefix == 'X') {
			digits = digits[2:]
		}
	}
	if len(digits) == 0 {
		return false
	}
	for _, digit := range digits {
		number := hermesYAMLDecimalDigit(digit)
		if base == 16 && digit >= 'a' && digit <= 'f' {
			number = int(digit-'a') + 10
		} else if base == 16 && digit >= 'A' && digit <= 'F' {
			number = int(digit-'A') + 10
		}
		if number < 0 || number >= base {
			return false
		}
	}
	return true
}

func hermesYAMLDecimalDigit(digit rune) int {
	if digit >= '0' && digit <= '9' {
		return int(digit - '0')
	}
	for _, interval := range unicode.Digit.R16 {
		if digit >= rune(interval.Lo) && digit <= rune(interval.Hi) && (digit-rune(interval.Lo))%rune(interval.Stride) == 0 {
			return int((digit-rune(interval.Lo))/rune(interval.Stride)) % 10
		}
	}
	for _, interval := range unicode.Digit.R32 {
		if digit >= rune(interval.Lo) && digit <= rune(interval.Hi) && (digit-rune(interval.Lo))%rune(interval.Stride) == 0 {
			return int((digit-rune(interval.Lo))/rune(interval.Stride)) % 10
		}
	}
	return -1
}

func hermesYAMLPythonFloatSyntax(value string) bool {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
		value = value[1:]
	}
	return value == "inf" || value == "infinity" || value == "nan" || hermesYAMLFloatSyntax.MatchString(value)
}

func hermesYAMLBinarySyntax(value string) bool {
	for _, char := range value {
		if char > 127 {
			return false
		}
	}
	position, padding := 0, 0
	for _, char := range value {
		if char == '=' {
			if position >= 2 {
				padding++
				if position+padding >= 4 {
					return true
				}
			}
		} else if char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '+' || char == '/' {
			padding = 0
			position = (position + 1) % 4
		}
	}
	return position == 0
}
