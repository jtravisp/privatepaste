package model

import "time"

// Paste is one stored paste. The server never sees plaintext: Ciphertext and
// IV are produced by the browser with AES-256-GCM and are opaque here.
type Paste struct {
	ID             string `dynamodbav:"id"`
	Ciphertext     string `dynamodbav:"ciphertext"`
	CreatedAt      int64  `dynamodbav:"created_at"`
	IV             string `dynamodbav:"iv"`
	BurnAfterRead  bool   `dynamodbav:"burn_after_read"`
	OwnerTokenHash string `dynamodbav:"owner_token_hash"`
	// TTL is a Unix timestamp DynamoDB uses to delete the item. Zero means
	// "never expires", and omitempty keeps the attribute off the item
	// entirely rather than storing 0.
	TTL int64 `dynamodbav:"ttl,omitempty"`
}

// Expired reports whether the paste is past its TTL. DynamoDB's TTL sweeper
// deletes expired items eventually -- typically within a few days -- so an
// expired item can still be returned by a read. The handler checks this so
// expiry is exact from the user's point of view.
func (p *Paste) Expired(now time.Time) bool {
	return p.TTL > 0 && now.Unix() >= p.TTL
}
