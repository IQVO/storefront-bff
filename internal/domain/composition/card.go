package composition

import "strings"

// cardFieldNames are JSON keys that only a raw card would carry. The storefront
// only ever handles a PSP payment-method token.
var cardFieldNames = map[string]bool{
	"pan": true, "cardnumber": true, "card_number": true, "cvc": true, "cvv": true,
	"cvc2": true, "securitycode": true, "security_code": true, "expiry": true,
	"expirymonth": true, "expiryyear": true, "cardholder": true,
}

// IsCardFieldName reports whether a JSON key looks like raw card data.
func IsCardFieldName(key string) bool { return cardFieldNames[strings.ToLower(key)] }

// LooksLikeCardNumber reports whether s is a 13-19 digit number that passes
// the Luhn check (spaces and dashes ignored): a primary account number must
// never be accepted where a token is expected.
func LooksLikeCardNumber(s string) bool {
	digits := make([]int, 0, 19)
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			digits = append(digits, int(r-'0'))
		case r == ' ' || r == '-':
		default:
			return false
		}
	}
	if len(digits) < 13 || len(digits) > 19 {
		return false
	}
	sum := 0
	for i := range digits {
		d := digits[len(digits)-1-i]
		if i%2 == 1 {
			if d *= 2; d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	return sum%10 == 0
}
