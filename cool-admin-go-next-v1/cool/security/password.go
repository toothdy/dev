package security

import "golang.org/x/crypto/bcrypt"

const passwordHashCost = 12

// 使用 bcrypt 生成密码摘要
func HashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), passwordHashCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

// 使用 bcrypt 校验密码摘要
func VerifyPassword(password string, hashed string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(password)) == nil
}
