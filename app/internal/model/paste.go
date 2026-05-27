package model

type Paste struct {
	ID             string `dynamodbav:"id"`
	Ciphertext     string `dynamodbav:"ciphertext"`
	CreatedAt      int64  `dynamodbav:"created_at"`
	IV             string `dynamodbav:"iv"`
	BurnAfterRead  bool   `dynamodbav:"burn_after_read"`
	TTL            int64  `dynamodbav:"ttl"`
	OwnerTokenHash string `dynamodbav:"owner_token_hash"`
}
