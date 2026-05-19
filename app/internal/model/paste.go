package model

type Paste struct {
	ID             string
	Ciphertext     string
	CreatedAt      int64
	IV             string
	BurnAfterRead  bool
	TTL            int64
	OwnerTokenHash string
}
