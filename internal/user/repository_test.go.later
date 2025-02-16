package user

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// MockCollection is a mock implementation of the mongo.Collection interface
type MockCollection struct {
	mock.Mock
}

func (m *MockCollection) FindOne(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
	args := m.Called(ctx, filter, opts)
	return args.Get(0).(*mongo.SingleResult)
}

func (m *MockCollection) UpdateOne(ctx context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
	args := m.Called(ctx, filter, update, opts)
	return args.Get(0).(*mongo.UpdateResult), args.Error(1)
}

// MockSingleResult is a mock implementation of the mongo.SingleResult interface
type MockSingleResult struct {
	mock.Mock
}

func (m *MockSingleResult) Decode(v interface{}) error {
	args := m.Called(v)
	return args.Error(0)
}

func TestFindUserByEmail(t *testing.T) {
	mockCollection := new(MockCollection)
	repo := &UserRepository{
		collection: mockCollection,
	}

	t.Run("user found", func(t *testing.T) {
		mockSingleResult := new(MockSingleResult)
		mockSingleResult.On("Decode", mock.Anything).Run(func(args mock.Arguments) {
			arg := args.Get(0).(*User)
			arg.ID = bson.NewObjectID()
			arg.Email = "test@example.com"
		}).Return(nil)

		mockCollection.On("FindOne", mock.Anything, mock.Anything, mock.Anything).Return(mockSingleResult)

		user, err := repo.FindUserByEmail(context.Background(), "test@example.com")
		assert.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, "test@example.com", user.Email)
	})

	t.Run("user not found", func(t *testing.T) {
		mockSingleResult := new(MockSingleResult)
		mockSingleResult.On("Decode", mock.Anything).Return(mongo.ErrNoDocuments)

		mockCollection.On("FindOne", mock.Anything, mock.Anything, mock.Anything).Return(mockSingleResult)

		user, err := repo.FindUserByEmail(context.Background(), "test@example.com")
		assert.NoError(t, err)
		assert.Nil(t, user)
	})
}

func TestUpsertUser(t *testing.T) {
	mockCollection := new(MockCollection)
	repo := &UserRepository{
		collection: mockCollection,
	}

	t.Run("insert new user", func(t *testing.T) {
		mockUpdateResult := &mongo.UpdateResult{
			UpsertedID: bson.NewObjectID(),
		}

		mockCollection.On("UpdateOne", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateResult, nil)

		user := &User{
			Email:        "test@example.com",
			Name:         "Test User",
			AvatarURL:    "http://example.com/avatar.png",
			HTMLURL:      "http://example.com",
			LastLoggedIn: time.Now(),
		}

		result, err := repo.UpsertUser(context.Background(), user)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.NotNil(t, result.UpsertedID)
	})

	t.Run("update existing user", func(t *testing.T) {
		mockUpdateResult := &mongo.UpdateResult{}

		mockCollection.On("UpdateOne", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateResult, nil)

		user := &User{
			ID:           bson.NewObjectID(),
			Email:        "test@example.com",
			Name:         "Test User",
			AvatarURL:    "http://example.com/avatar.png",
			HTMLURL:      "http://example.com",
			LastLoggedIn: time.Now(),
		}

		result, err := repo.UpsertUser(context.Background(), user)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Nil(t, result.UpsertedID)
	})
}
