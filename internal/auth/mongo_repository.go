package auth

import (
	"context"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoConfig contains connection settings for MongoDB user repository.
type MongoConfig struct {
	URI        string // e.g. mongodb://localhost:27017
	Database   string // e.g. blockverse
	Collection string // e.g. users
	Counters   string // e.g. counters (for auto-increment)
}

// MongoUserRepo implements UserRepository on MongoDB backend.
type MongoUserRepo struct {
	client      *mongo.Client
	collection  *mongo.Collection
	counterColl *mongo.Collection
	ctxTimeout  time.Duration
}

// NewMongoUserRepo establishes connection and returns repository.
func NewMongoUserRepo(cfg MongoConfig) (*MongoUserRepo, error) {
	if cfg.URI == "" {
		cfg.URI = "mongodb://localhost:27017"
	}
	if cfg.Database == "" {
		cfg.Database = "blockverse"
	}
	if cfg.Collection == "" {
		cfg.Collection = "users"
	}
	if cfg.Counters == "" {
		cfg.Counters = "counters"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.URI))
	if err != nil {
		return nil, err
	}
	// ping
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}
	db := client.Database(cfg.Database)
	repo := &MongoUserRepo{
		client:      client,
		collection:  db.Collection(cfg.Collection),
		counterColl: db.Collection(cfg.Counters),
		ctxTimeout:  5 * time.Second,
	}

	// Ensure indexes
	if err := repo.ensureIndexes(); err != nil {
		return nil, err
	}
	return repo, nil
}

func (m *MongoUserRepo) ensureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), m.ctxTimeout)
	defer cancel()
	usernameIdx := mongo.IndexModel{
		Keys:    bson.D{{Key: "username", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("username_unique"),
	}
	userIDIdx := mongo.IndexModel{
		Keys:    bson.D{{Key: "user_id", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("userid_unique"),
	}
	_, err := m.collection.Indexes().CreateMany(ctx, []mongo.IndexModel{usernameIdx, userIDIdx})
	return err
}

// GetUserByUsername implements UserRepository.
func (m *MongoUserRepo) GetUserByUsername(username string) (*User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), m.ctxTimeout)
	defer cancel()
	lower := strings.ToLower(username)
	filter := bson.M{"username": lower}
	var doc struct {
		UserID       uint64    `bson:"user_id"`
		Username     string    `bson:"username"`
		PasswordHash string    `bson:"password_hash"`
		IsAdmin      bool      `bson:"is_admin"`
		CreatedAt    time.Time `bson:"created_at"`
		LastLogin    time.Time `bson:"last_login"`
	}
	err := m.collection.FindOne(ctx, filter).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &User{
		ID:           doc.UserID,
		Username:     doc.Username,
		PasswordHash: doc.PasswordHash,
		CreatedAt:    doc.CreatedAt,
		LastLogin:    doc.LastLogin,
		IsAdmin:      doc.IsAdmin,
	}, nil
}

// CreateUser inserts a new document and returns created user.
func (m *MongoUserRepo) CreateUser(username string, passwordHash string, isAdmin bool) (*User, error) {
	lower := strings.ToLower(username)

	// generate next id
	nextID, err := m.nextSequence("userid")
	if err != nil {
		return nil, err
	}
	user := &User{
		ID:           nextID,
		Username:     lower,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now(),
		LastLogin:    time.Now(),
		IsAdmin:      isAdmin,
	}
	ctx, cancel := context.WithTimeout(context.Background(), m.ctxTimeout)
	defer cancel()
	_, err = m.collection.InsertOne(ctx, bson.M{
		"user_id":       user.ID,
		"username":      user.Username,
		"password_hash": user.PasswordHash,
		"is_admin":      user.IsAdmin,
		"created_at":    user.CreatedAt,
		"last_login":    user.LastLogin,
	})
	if mongo.IsDuplicateKeyError(err) {
		return nil, ErrUserExists
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// nextSequence atomically increments a counter and returns new value.
func (m *MongoUserRepo) nextSequence(name string) (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), m.ctxTimeout)
	defer cancel()
	res := m.counterColl.FindOneAndUpdate(ctx,
		bson.M{"_id": name},
		bson.M{"$inc": bson.M{"seq": 1}},
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After),
	)
	var doc struct {
		Seq int64 `bson:"seq"`
	}
	err := res.Decode(&doc)
	if err != nil {
		return 0, err
	}
	return uint64(doc.Seq), nil
}

// Close terminates connection.
func (m *MongoUserRepo) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return m.client.Disconnect(ctx)
}
