package auth

import "golang.org/x/crypto/bcrypt"

type Hasher interface {
	Hash(password string) (string, error)
	Compare(hash, password string) error
}

type hasher struct {
	cost int
}

func NewHasher(cost int) Hasher {
	return &hasher{cost: cost}
}

func (h *hasher) Hash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		h.cost,
	)
	if err != nil {
		return "", err
	}

	return string(hash), nil
}

func (h *hasher) Compare(hash, password string) error {
	return bcrypt.CompareHashAndPassword(
		[]byte(hash),
		[]byte(password),
	)
}
