package auth

import "github.com/pquerna/otp/totp"

func GenerateTOTPSecret(issuer, accountEmail string) (secret string, otpAuthURL string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: accountEmail,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

func ValidateTOTP(secret, code string) bool {
	return totp.Validate(code, secret)
}
