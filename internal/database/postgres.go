// internal/database/postgres.go
package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"gator-swamp/internal/models"
	"gator-swamp/internal/utils"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// DBAdapter defines the common interface for database operations.
// It allows using PostgreSQL as the backend.
type DBAdapter interface {
	// Connection
	Close(ctx context.Context) error

	// User methods
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)
	GetUser(ctx context.Context, id uuid.UUID) (*models.User, error)
	SaveUser(ctx context.Context, user *models.User) error
	UpdateUserActivity(ctx context.Context, id uuid.UUID, active bool) error
	UpdateUserSubreddits(ctx context.Context, userID uuid.UUID, subID uuid.UUID, join bool) error
	GetAllUsers(ctx context.Context) ([]*models.User, error)
	// TODO: Consider adding UpdateUserKarma directly?

	// Subreddit methods
	CreateSubreddit(ctx context.Context, sub *models.Subreddit) error
	GetSubredditByID(ctx context.Context, id uuid.UUID) (*models.Subreddit, error)
	GetSubredditByName(ctx context.Context, name string) (*models.Subreddit, error)
	GetAllSubreddits(ctx context.Context) ([]*models.Subreddit, error)
	UpdateSubredditMemberCount(ctx context.Context, subID uuid.UUID, delta int) error
	GetSubredditMemberIDs(ctx context.Context, subredditID uuid.UUID) ([]uuid.UUID, error)

	// Post methods
	SavePost(ctx context.Context, post *models.Post) error
	GetPost(ctx context.Context, postID uuid.UUID, requestingUserID uuid.UUID) (*models.Post, error)
	RecordVote(ctx context.Context, userID, contentID uuid.UUID, contentType models.VoteContentType, direction models.VoteDirection) error
	GetRecentPosts(ctx context.Context, limit, offset int, requestingUserID uuid.UUID) ([]*models.Post, error)
	GetUserFeed(ctx context.Context, userID uuid.UUID, limit, offset int, requestingUserID uuid.UUID) ([]*models.Post, error)
	GetPostsBySubreddit(ctx context.Context, subredditID uuid.UUID, limit int, offset int) ([]*models.Post, error)
	GetAllPosts(ctx context.Context) ([]*models.Post, error)

	// Comment methods
	SaveComment(ctx context.Context, comment *models.Comment) error
	GetComment(ctx context.Context, id uuid.UUID) (*models.Comment, error)
	GetPostComments(ctx context.Context, postID uuid.UUID, requestingUserID uuid.UUID) ([]*models.Comment, error)
	DeleteCommentAndDecrementCount(ctx context.Context, commentID uuid.UUID) error
	// UpdateCommentVotes(ctx context.Context, commentID uuid.UUID, upvotes int, downvotes int) error // Replaced by RecordVote
	GetAllComments(ctx context.Context) ([]*models.Comment, error) // For handleLoadComments

	// Message methods
	SaveMessage(ctx context.Context, msg *models.DirectMessage) error
	GetMessagesByUser(ctx context.Context, userID uuid.UUID) ([]*models.DirectMessage, error)
	UpdateMessageStatus(ctx context.Context, msgID uuid.UUID, isRead *bool, isDeleted *bool) error
}

// PostgresDB represents a PostgreSQL database connection
type PostgresDB struct {
	DB *sqlx.DB
}

// NewPostgresDB creates a new PostgreSQL database connection
func NewPostgresDB(connectionString string) (*PostgresDB, error) {
	db, err := sqlx.Connect("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %v", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Ping the database to verify connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping PostgreSQL: %v", err)
	}

	log.Println("Successfully connected to PostgreSQL!")

	return &PostgresDB{
		DB: db,
	}, nil
}

// Close closes the database connection
func (p *PostgresDB) Close(ctx context.Context) error {
	log.Println("Closing PostgreSQL connection...")
	return p.DB.Close()
}

// InitializeTables creates all necessary tables if they don't exist
func (p *PostgresDB) InitializeTables(ctx context.Context) error {
	// Users table
	_, err := p.DB.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY,
			username VARCHAR(50) UNIQUE NOT NULL,
			email VARCHAR(100) UNIQUE NOT NULL,
			password_hash VARCHAR(100) NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			karma INTEGER DEFAULT 0,
			is_connected BOOLEAN DEFAULT FALSE NOT NULL,
			last_active TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			bio TEXT,
			profile_image VARCHAR(255)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create users table: %v", err)
	}

	// Subreddits table
	_, err = p.DB.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS subreddits (
			id UUID PRIMARY KEY,
			name VARCHAR(50) UNIQUE NOT NULL,
			description TEXT,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			created_by UUID REFERENCES users(id),
			rules JSONB,
			member_count INTEGER DEFAULT 0
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create subreddits table: %v", err)
	}

	// Subreddit members table
	_, err = p.DB.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS subreddit_members (
			subreddit_id UUID REFERENCES subreddits(id),
			user_id UUID REFERENCES users(id),
			joined_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			PRIMARY KEY (subreddit_id, user_id)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create subreddit_members table: %v", err)
	}

	// Posts table
	_, err = p.DB.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS posts (
			id UUID PRIMARY KEY,
			title VARCHAR(300) NOT NULL,
			content TEXT,
			author_id UUID REFERENCES users(id),
			subreddit_id UUID REFERENCES subreddits(id),
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			karma INTEGER DEFAULT 0,
			upvotes INTEGER DEFAULT 0,
			downvotes INTEGER DEFAULT 0,
			comment_count INTEGER DEFAULT 0
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create posts table: %v", err)
	}

	// Comments table
	_, err = p.DB.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS comments (
			id UUID PRIMARY KEY,
			content TEXT NOT NULL,
			author_id UUID REFERENCES users(id),
			post_id UUID REFERENCES posts(id),
			parent_id UUID REFERENCES comments(id),
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			karma INTEGER DEFAULT 0,
			upvotes INTEGER DEFAULT 0,
			downvotes INTEGER DEFAULT 0
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create comments table: %v", err)
	}

	// Votes table
	_, err = p.DB.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS votes (
			id UUID PRIMARY KEY,
			user_id UUID REFERENCES users(id),
			content_id UUID NOT NULL,
			content_type VARCHAR(20) NOT NULL,
			vote_type INTEGER NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			UNIQUE(user_id, content_id, content_type)
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create votes table: %v", err)
	}

	// Messages table
	_, err = p.DB.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS messages (
			id UUID PRIMARY KEY,
			sender_id UUID REFERENCES users(id),
			receiver_id UUID REFERENCES users(id),
			content TEXT NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			read_at TIMESTAMP WITH TIME ZONE
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create messages table: %v", err)
	}

	return nil
}

// GetUserByEmail fetches a user by their email address.
func (p *PostgresDB) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `SELECT id, username, email, password_hash, karma, created_at, updated_at, is_connected, last_active FROM users WHERE email = $1`
	var user models.User
	err := p.DB.GetContext(ctx, &user, query, email)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, utils.NewAppError(utils.ErrNotFound, "user not found", err)
		}
		return nil, utils.NewAppError(utils.ErrDatabase, "failed to query user by email", err)
	}
	return &user, nil
}

// GetUser fetches a user by their ID.
func (p *PostgresDB) GetUser(ctx context.Context, id uuid.UUID) (*models.User, error) {
	// First fetch basic user info
	query := `SELECT id, username, email, password_hash, karma, created_at, updated_at, is_connected, last_active FROM users WHERE id = $1`
	var user models.User
	err := p.DB.GetContext(ctx, &user, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, utils.NewAppError(utils.ErrNotFound, "user not found", err)
		}
		return nil, utils.NewAppError(utils.ErrDatabase, "failed to query user by id", err)
	}

	// Now fetch subreddit memberships from the subreddit_members table
	membershipQuery := `SELECT subreddit_id FROM subreddit_members WHERE user_id = $1`
	var subredditIDs []uuid.UUID
	err = p.DB.SelectContext(ctx, &subredditIDs, membershipQuery, id)
	if err != nil {
		return nil, utils.NewAppError(utils.ErrDatabase, "failed to query user subreddit memberships", err)
	}

	// Add subreddit IDs to the user object
	user.Subreddits = subredditIDs

	return &user, nil
}

// SaveUser inserts a new user into the database.
func (p *PostgresDB) SaveUser(ctx context.Context, user *models.User) error {
	// Ensure UpdatedAt and CreatedAt are set
	now := time.Now()
	user.UpdatedAt = now
	if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}
	// Set LastActive if it's zero (likely a new user)
	if user.LastActive.IsZero() {
		user.LastActive = now // Default last active to creation time
	}

	query := `
		INSERT INTO users (id, username, email, password_hash, karma, created_at, updated_at, is_connected, last_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := p.DB.ExecContext(ctx, query,
		user.ID,
		user.Username,
		user.Email,
		user.HashedPassword,
		user.Karma,
		user.CreatedAt,
		user.UpdatedAt,
		user.IsConnected,
		user.LastActive,
	)

	if err != nil {
		// Check for duplicate key violation (username or email)
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code.Name() == "unique_violation" {
			return utils.NewAppError(utils.ErrDuplicate, fmt.Sprintf("user already exists: %v", pqErr.Constraint), err)
		}
		return utils.NewAppError(utils.ErrDatabase, "failed to save user", err)
	}
	return nil
}

// UpdateUserActivity updates the user's last active time and connection status.
func (p *PostgresDB) UpdateUserActivity(ctx context.Context, id uuid.UUID, active bool) error {
	query := `UPDATE users SET last_active = NOW(), is_connected = $1, updated_at = NOW() WHERE id = $2`
	result, err := p.DB.ExecContext(ctx, query, active, id)
	if err != nil {
		return utils.NewAppError(utils.ErrDatabase, "failed to update user activity", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return utils.NewAppError(utils.ErrDatabase, "failed to get rows affected after update", err)
	}
	if rowsAffected == 0 {
		return utils.NewAppError(utils.ErrNotFound, "user not found for activity update", nil)
	}
	return nil
}

// UpdateUserSubreddits adds or removes a subreddit subscription for a user.
func (p *PostgresDB) UpdateUserSubreddits(ctx context.Context, userID uuid.UUID, subID uuid.UUID, join bool) error {
	var query string
	var err error

	if join {
		// Add user to subreddit members
		query = `INSERT INTO subreddit_members (user_id, subreddit_id, joined_at) VALUES ($1, $2, NOW()) ON CONFLICT (user_id, subreddit_id) DO NOTHING`
		_, err = p.DB.ExecContext(ctx, query, userID, subID)
	} else {
		// Remove user from subreddit members
		query = `DELETE FROM subreddit_members WHERE user_id = $1 AND subreddit_id = $2`
		_, err = p.DB.ExecContext(ctx, query, userID, subID)
		// Note: DELETE doesn't error if the row doesn't exist, which is fine.
	}

	if err != nil {
		// TODO: More specific error handling? (e.g., foreign key constraint violation)
		return utils.NewAppError(utils.ErrDatabase, "failed to update user subreddit membership", err)
	}
	return nil
}

// GetAllUsers fetches all users from the database.
func (p *PostgresDB) GetAllUsers(ctx context.Context) ([]*models.User, error) {
	query := `SELECT id, username, email, password_hash, karma, created_at, updated_at, is_connected, last_active FROM users ORDER BY created_at DESC`
	users := []*models.User{}
	err := p.DB.SelectContext(ctx, &users, query)
	if err != nil {
		return nil, utils.NewAppError(utils.ErrDatabase, "failed to query all users", err)
	}
	return users, nil
}

// --- Subreddit Methods ---

// CreateSubreddit inserts a new subreddit record.
func (p *PostgresDB) CreateSubreddit(ctx context.Context, sub *models.Subreddit) error {
	// Ensure CreatedAt is set if zero
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = time.Now()
	}
	// Ensure Members (member_count) is at least 0
	if sub.Members < 0 {
		sub.Members = 0
	}

	query := `
		INSERT INTO subreddits (id, name, description, created_by, member_count, created_at)
		VALUES (:id, :name, :description, :created_by, :member_count, :created_at)
	`
	_, err := p.DB.NamedExecContext(ctx, query, sub)
	if err != nil {
		// TODO: Check for unique constraint violation (e.g., pq error code 23505)
		// and potentially return utils.ErrDuplicate
		return utils.NewAppError(utils.ErrDatabase, "failed to create subreddit", err)
	}
	return nil
}

// GetSubredditByID fetches a subreddit by its ID.
func (p *PostgresDB) GetSubredditByID(ctx context.Context, id uuid.UUID) (*models.Subreddit, error) {
	query := `SELECT id, name, description, created_by, member_count, created_at FROM subreddits WHERE id = $1`
	var sub models.Subreddit
	err := p.DB.GetContext(ctx, &sub, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, utils.NewAppError(utils.ErrNotFound, "subreddit not found", err)
		}
		return nil, utils.NewAppError(utils.ErrDatabase, "failed to query subreddit by id", err)
	}
	return &sub, nil
}

// GetSubredditByName fetches a subreddit by its name.
func (p *PostgresDB) GetSubredditByName(ctx context.Context, name string) (*models.Subreddit, error) {
	query := `SELECT id, name, description, created_by, member_count, created_at FROM subreddits WHERE name = $1`
	var sub models.Subreddit
	err := p.DB.GetContext(ctx, &sub, query, name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, utils.NewAppError(utils.ErrNotFound, "subreddit not found", err)
		}
		return nil, utils.NewAppError(utils.ErrDatabase, "failed to query subreddit by name", err)
	}
	return &sub, nil
}

// GetAllSubreddits fetches all subreddit records.
func (p *PostgresDB) GetAllSubreddits(ctx context.Context) ([]*models.Subreddit, error) {
	query := `SELECT id, name, description, created_by, member_count, created_at FROM subreddits ORDER BY created_at DESC`
	var subs []*models.Subreddit
	err := p.DB.SelectContext(ctx, &subs, query)
	if err != nil {
		// For Select, ErrNoRows is not returned for zero rows, so we just check for other errors.
		return nil, utils.NewAppError(utils.ErrDatabase, "failed to query all subreddits", err)
	}
	// Return empty slice if no rows found, not nil
	if subs == nil {
		subs = make([]*models.Subreddit, 0)
	}
	return subs, nil
}

// UpdateSubredditMemberCount adjusts the member_count of a subreddit.
func (p *PostgresDB) UpdateSubredditMemberCount(ctx context.Context, subID uuid.UUID, delta int) error {
	query := `UPDATE subreddits SET member_count = member_count + $1 WHERE id = $2`
	result, err := p.DB.ExecContext(ctx, query, delta, subID)
	if err != nil {
		return utils.NewAppError(utils.ErrDatabase, "failed to update subreddit member count", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return utils.NewAppError(utils.ErrNotFound, "subreddit not found when updating member count", nil)
	}
	return nil
}

// GetSubredditMemberIDs fetches all member IDs for a given subreddit.
func (p *PostgresDB) GetSubredditMemberIDs(ctx context.Context, subredditID uuid.UUID) ([]uuid.UUID, error) {
	query := `SELECT user_id FROM subreddit_members WHERE subreddit_id = $1`
	var memberIDs []uuid.UUID
	err := p.DB.SelectContext(ctx, &memberIDs, query, subredditID)
	if err != nil {
		return nil, utils.NewAppError(utils.ErrDatabase, "failed to query subreddit member IDs", err)
	}
	return memberIDs, nil
}

// --- Post Methods ---

// SavePost inserts a new post or updates an existing one based on the ID.
func (p *PostgresDB) SavePost(ctx context.Context, post *models.Post) error {
	// Ensure timestamps are set
	post.UpdatedAt = time.Now()
	if post.CreatedAt.IsZero() {
		post.CreatedAt = post.UpdatedAt
	}

	query := `
		INSERT INTO posts (id, title, content, author_id, subreddit_id, karma, comment_count, created_at, updated_at)
		VALUES (:id, :title, :content, :author_id, :subreddit_id, :karma, :comment_count, :created_at, :updated_at)
		ON CONFLICT (id) DO UPDATE SET
			title = EXCLUDED.title,
			content = EXCLUDED.content,
			karma = EXCLUDED.karma,
			comment_count = EXCLUDED.comment_count,
			updated_at = EXCLUDED.updated_at
	`
	// Note: We don't update author_id or subreddit_id on conflict

	_, err := p.DB.NamedExecContext(ctx, query, post)
	if err != nil {
		return utils.NewAppError(utils.ErrDatabase, "failed to save post", err)
	}
	return nil
}

// GetPost fetches a post by its ID and includes the requesting user's vote status.
func (p *PostgresDB) GetPost(ctx context.Context, postID uuid.UUID, requestingUserID uuid.UUID) (*models.Post, error) {
	query := `SELECT 
			p.id, p.title, p.content, p.author_id, p.subreddit_id, p.karma, 
			p.upvotes, p.downvotes, p.comment_count, p.created_at, p.updated_at,
			u.username as author_username, -- Join to get author username
			s.name as subreddit_name      -- Join to get subreddit name
		FROM posts p
		LEFT JOIN users u ON p.author_id = u.id
		LEFT JOIN subreddits s ON p.subreddit_id = s.id
		WHERE p.id = $1`
	var post models.Post
	err := p.DB.GetContext(ctx, &post, query, postID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, utils.NewAppError(utils.ErrNotFound, "post not found", err)
		}
		log.Printf("Error fetching post %s: %v", postID, err) // Log detailed error
		return nil, utils.NewAppError(utils.ErrDatabase, "failed to query post by id", err)
	}

	// If a requesting user ID is provided and valid, fetch their vote status
	if requestingUserID != uuid.Nil {
		// Expect string type based on error logs (e.g., "up", "down")
		var voteType sql.NullString
		voteQuery := `SELECT vote_type FROM votes WHERE user_id = $1 AND content_id = $2 AND content_type = $3`
		err = p.DB.GetContext(ctx, &voteType, voteQuery, requestingUserID, postID, string(models.PostVote))

		if err != nil && err != sql.ErrNoRows {
			log.Printf("Error fetching vote status for user %s on post %s: %v", requestingUserID, postID, err)
			// Don't fail the whole request, just log the error and return post without vote status
		} else if err == nil && voteType.Valid {
			// Check the string value
			if voteType.String == "up" || voteType.String == "1" { // Check for string "up" or legacy "1"
				up := "up"
				post.CurrentUserVote = &up
			} else if voteType.String == "down" || voteType.String == "-1" { // Check for string "down" or legacy "-1"
				down := "down"
				post.CurrentUserVote = &down
			}
			// If voteType string is something else (e.g., "0", empty), CurrentUserVote remains nil
		}
		// If err == sql.ErrNoRows, CurrentUserVote remains nil (no vote)
	}

	// The rest of the post fields (like AuthorUsername, SubredditName) should be populated by the GetContext query now
	return &post, nil
}

// RecordVote handles inserting, updating, or deleting a vote record
// and updating the corresponding karma for the content and its author.
func (p *PostgresDB) RecordVote(ctx context.Context, userID, contentID uuid.UUID, contentType models.VoteContentType, direction models.VoteDirection) error {
	tx, err := p.DB.BeginTxx(ctx, nil)
	if err != nil {
		return utils.NewAppError(utils.ErrDatabase, "failed to begin transaction", err)
	}
	defer tx.Rollback() // Rollback is ignored if tx is committed.

	var previousVoteType models.VoteDirection
	var existingVoteID uuid.UUID // Needed if we need to update/delete
	var authorID uuid.UUID

	// --- 1. Determine previous vote and content author ---
	getVoteQuery := `SELECT id, vote_type FROM votes WHERE user_id = $1 AND content_id = $2 AND content_type = $3`
	err = tx.QueryRowxContext(ctx, getVoteQuery, userID, contentID, contentType).Scan(&existingVoteID, &previousVoteType)
	if err != nil && err != sql.ErrNoRows {
		return utils.NewAppError(utils.ErrDatabase, "failed to check existing vote", err)
	}
	// If err == sql.ErrNoRows, previousVoteType remains empty (zero value)

	// Get author ID based on content type
	var getAuthorQuery string
	if contentType == models.PostVote {
		getAuthorQuery = `SELECT author_id FROM posts WHERE id = $1`
	} else if contentType == models.CommentVote {
		getAuthorQuery = `SELECT author_id FROM comments WHERE id = $1`
	} else {
		return utils.NewAppError(utils.ErrInvalidInput, "invalid content type for voting", nil)
	}

	err = tx.QueryRowxContext(ctx, getAuthorQuery, contentID).Scan(&authorID)
	if err != nil {
		if err == sql.ErrNoRows {
			// Content might have been deleted, or author set to NULL
			log.Printf("Warning: Could not find author for content %s (%s) during vote recording", contentID, contentType)
			// Proceed without author karma update if author is not found or null
			authorID = uuid.Nil
		} else {
			return utils.NewAppError(utils.ErrDatabase, "failed to get content author", err)
		}
	}

	// --- 2. Calculate Karma Delta ---
	karmaDelta := 0
	upvoteDelta := 0
	downvoteDelta := 0
	switch direction {
	case models.VoteUp:
		if previousVoteType == models.VoteDown {
			karmaDelta = 2
			upvoteDelta = 1
			downvoteDelta = -1
		} else if previousVoteType != models.VoteUp { // No vote or different vote
			karmaDelta = 1
			upvoteDelta = 1
			// downvoteDelta remains 0
		}
	case models.VoteDown:
		if previousVoteType == models.VoteUp {
			karmaDelta = -2
			upvoteDelta = -1
			downvoteDelta = 1
		} else if previousVoteType != models.VoteDown { // No vote or different vote
			karmaDelta = -1
			// upvoteDelta remains 0
			downvoteDelta = 1
		}
	case models.VoteNone: // Removing vote
		if previousVoteType == models.VoteUp {
			karmaDelta = -1
			upvoteDelta = -1
			// downvoteDelta remains 0
		} else if previousVoteType == models.VoteDown {
			karmaDelta = 1
			// upvoteDelta remains 0
			downvoteDelta = -1
		}
	default:
		return utils.NewAppError(utils.ErrInvalidInput, "invalid vote direction", nil)
	}

	// --- 3. Update Content and Author Karma/Votes if Deltas are non-zero ---
	// Only proceed if there's a change in karma, upvotes, or downvotes
	if karmaDelta != 0 || upvoteDelta != 0 || downvoteDelta != 0 {
		var updateContentQuery string
		if contentType == models.PostVote {
			updateContentQuery = `UPDATE posts SET karma = karma + $1, upvotes = upvotes + $2, downvotes = downvotes + $3, updated_at = NOW() WHERE id = $4`
		} else { // CommentVote
			updateContentQuery = `UPDATE comments SET karma = karma + $1, upvotes = upvotes + $2, downvotes = downvotes + $3, updated_at = NOW() WHERE id = $4`
		}
		_, err = tx.ExecContext(ctx, updateContentQuery, karmaDelta, upvoteDelta, downvoteDelta, contentID)
		if err != nil {
			return utils.NewAppError(utils.ErrDatabase, "failed to update content karma/votes", err)
		}

		// Update author's karma (upvotes/downvotes are not tracked on the user model)
		if authorID != uuid.Nil && karmaDelta != 0 { // Only update author karma if it changed
			updateAuthorKarmaQuery := `UPDATE users SET karma = karma + $1, updated_at = NOW() WHERE id = $2`
			_, err = tx.ExecContext(ctx, updateAuthorKarmaQuery, karmaDelta, authorID)
			if err != nil {
				log.Printf("Warning: Failed to update author (%s) karma during vote: %v", authorID, err)
			}
		}
	}

	// --- 4. Update or Delete Vote Record ---
	if direction == models.VoteNone {
		// Delete the vote record if it exists
		if previousVoteType != "" { // Only delete if there was a previous vote
			deleteQuery := `DELETE FROM votes WHERE id = $1`
			_, err = tx.ExecContext(ctx, deleteQuery, existingVoteID)
			if err != nil {
				return utils.NewAppError(utils.ErrDatabase, "failed to delete vote record", err)
			}
		}
	} else {
		// Insert or Update the vote record
		upsertQuery := `
			INSERT INTO votes (id, user_id, content_id, content_type, vote_type, created_at)
			VALUES ($1, $2, $3, $4, $5, NOW())
			ON CONFLICT (user_id, content_id, content_type) DO UPDATE SET
				vote_type = EXCLUDED.vote_type,
				created_at = NOW() -- Update timestamp on change
		`
		// Use existingVoteID if known, otherwise generate a new one
		voteID := existingVoteID
		if voteID == uuid.Nil {
			voteID = uuid.New() // Generate new ID for insertion
		}

		_, err = tx.ExecContext(ctx, upsertQuery, voteID, userID, contentID, contentType, direction)
		if err != nil {
			return utils.NewAppError(utils.ErrDatabase, "failed to upsert vote record", err)
		}
	}

	// --- 5. Commit Transaction ---
	err = tx.Commit()
	if err != nil {
		return utils.NewAppError(utils.ErrDatabase, "failed to commit vote transaction", err)
	}

	return nil
}

// GetRecentPosts retrieves the most recent posts across all subreddits, including the requesting user's vote status.
func (p *PostgresDB) GetRecentPosts(ctx context.Context, limit, offset int, requestingUserID uuid.UUID) ([]*models.Post, error) {
	// Temporary struct to handle scanning potential string or int vote_type
	type ScanPost struct {
		models.Post
		RawVoteType sql.NullString `db:"current_user_vote"` // Use NullString like in GetPost
	}

	query := `
		SELECT 
		    p.id, p.title, p.content, p.author_id, u.username AS author_username, 
		    p.subreddit_id, s.name AS subreddit_name, 
		    p.created_at, p.updated_at, p.karma, p.upvotes, p.downvotes, p.comment_count,
		    v.vote_type AS current_user_vote -- Select the raw vote_type (might be string or int)
		FROM posts p
		JOIN users u ON p.author_id = u.id
		JOIN subreddits s ON p.subreddit_id = s.id
		LEFT JOIN votes v ON v.content_id = p.id AND v.user_id = $3 AND v.content_type = 'post'
		ORDER BY p.created_at DESC
		LIMIT $1 OFFSET $2
	`

	scannedPosts := []ScanPost{}
	err := p.DB.SelectContext(ctx, &scannedPosts, query, limit, offset, requestingUserID)

	if err != nil {
		log.Printf("Error querying recent posts: %v", err)
		return nil, utils.NewAppError(utils.ErrDatabase, "failed to query recent posts", err)
	}

	// Process scanned posts to populate CurrentUserVote string pointer correctly
	posts := make([]*models.Post, len(scannedPosts))
	for i, sp := range scannedPosts {
		post := sp.Post // Create a copy
		if sp.RawVoteType.Valid {
			// Check string value like in GetPost workaround
			voteStr := sp.RawVoteType.String
			if voteStr == "up" || voteStr == "1" { // Check for "up" or legacy "1"
				up := "up"
				post.CurrentUserVote = &up
			} else if voteStr == "down" || voteStr == "-1" { // Check for "down" or legacy "-1"
				down := "down"
				post.CurrentUserVote = &down
			}
			// If voteStr is something else (e.g., "0", empty), CurrentUserVote remains nil
		}
		posts[i] = &post // Assign the processed post pointer
	}

	return posts, nil
}

// GetUserFeed retrieves posts from subreddits the user is subscribed to, ordered by creation date.
// It now also fetches the requesting user's vote status for each post.
func (p *PostgresDB) GetUserFeed(ctx context.Context, userID uuid.UUID, limit, offset int, requestingUserID uuid.UUID) ([]*models.Post, error) {
	// 1. Get subscribed subreddit IDs
	var subscribedIDs []uuid.UUID
	subQuery := `SELECT subreddit_id FROM subreddit_members WHERE user_id = $1`
	err := p.DB.SelectContext(ctx, &subscribedIDs, subQuery, userID)
	if err != nil {
		return nil, utils.NewAppError(utils.ErrDatabase, "failed to query user subscriptions", err)
	}

	if len(subscribedIDs) == 0 {
		return []*models.Post{}, nil // User is not subscribed to any subreddits
	}

	// 2. Get posts from those subreddits, including vote status
	query, args, err := sqlx.In(`
		SELECT 
		    p.id, p.title, p.content, p.author_id, u.username AS author_username, 
		    p.subreddit_id, s.name AS subreddit_name, 
		    p.created_at, p.updated_at, p.karma, p.upvotes, p.downvotes, p.comment_count,
		    v.vote_type AS current_user_vote
		FROM posts p
		JOIN users u ON p.author_id = u.id
		JOIN subreddits s ON p.subreddit_id = s.id
		LEFT JOIN votes v ON v.content_id = p.id AND v.user_id = ? AND v.content_type = 'post' -- Placeholder for requestingUserID
		WHERE p.subreddit_id IN (?)
		ORDER BY p.created_at DESC
		LIMIT ? OFFSET ?
	`, requestingUserID, subscribedIDs, limit, offset)

	if err != nil {
		return nil, utils.NewAppError(utils.ErrDatabase, "failed to build feed query with votes", err)
	}

	query = p.DB.Rebind(query) // Rebind ? to $1, $2, etc. for PostgreSQL

	posts := []*models.Post{}
	err = p.DB.SelectContext(ctx, &posts, query, args...)
	if err != nil {
		log.Printf("Error querying user feed posts: %v, Query: %s, Args: %v", err, query, args)
		return nil, utils.NewAppError(utils.ErrDatabase, "failed to query user feed posts", err)
	}

	// Post-process vote status (same as GetRecentPosts)
	for _, post := range posts {
		if post.CurrentUserVote != nil {
			vote := *post.CurrentUserVote
			if vote != "up" && vote != "down" {
				post.CurrentUserVote = nil
			}
		}
	}

	return posts, nil
}

// GetPostsBySubreddit retrieves posts for a specific subreddit with pagination.
// TODO: Add requestingUserID to GetPostsBySubreddit to fetch currentUserVote.
func (p *PostgresDB) GetPostsBySubreddit(ctx context.Context, subredditID uuid.UUID, limit int, offset int) ([]*models.Post, error) {
	query := `
		SELECT id, title, content, author_id, subreddit_id, created_at, updated_at, karma, upvotes, downvotes, comment_count
		FROM posts
		WHERE subreddit_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	posts := []*models.Post{}
	err := p.DB.SelectContext(ctx, &posts, query, subredditID, limit, offset)
	if err != nil {
		return nil, utils.NewAppError(utils.ErrDatabase, "failed to query posts by subreddit", err)
	}
	return posts, nil
}

// GetAllPosts retrieves all posts, ordered by creation date.
func (p *PostgresDB) GetAllPosts(ctx context.Context) ([]*models.Post, error) {
	// Warning: Loading ALL posts might be memory-intensive for large datasets.
	// Consider pagination or alternative loading strategies if needed.
	query := `SELECT id, title, content, author_id, subreddit_id, created_at, updated_at, karma, upvotes, downvotes, comment_count
	          FROM posts
	          ORDER BY created_at DESC`
	posts := []*models.Post{}
	err := p.DB.SelectContext(ctx, &posts, query)
	if err != nil {
		return nil, utils.NewAppError(utils.ErrDatabase, "failed to query all posts", err)
	}
	if posts == nil {
		posts = make([]*models.Post, 0)
	}
	return posts, nil
}

// --- Comment Methods ---

// SaveComment inserts a new comment or updates an existing one.
// It now also increments the comment_count on the associated post in a transaction.
func (p *PostgresDB) SaveComment(ctx context.Context, comment *models.Comment) error {
	tx, err := p.DB.BeginTxx(ctx, nil)
	if err != nil {
		return utils.NewAppError(utils.ErrDatabase, "failed to begin transaction for save comment", err)
	}
	// Defers will not run if panic occurs, but Rollback is safe to call multiple times.
	// We will explicitly call Rollback on error and Commit on success.

	comment.UpdatedAt = time.Now()
	if comment.CreatedAt.IsZero() {
		comment.CreatedAt = comment.UpdatedAt
	}

	// Add log just before DB execution
	log.Printf("Saving comment ID %s. ParentID: %v, PostID: %s", comment.ID, comment.ParentID, comment.PostID)

	// Determine if it's a new comment for the purpose of incrementing post's comment_count.
	// A more robust way would be to check if the comment ID already exists, but for now,
	// we assume if it's not an update (e.g. content change), it's new for counting purposes.
	// For simplicity, we'll always try to increment if the main save succeeds and it's not an 'is_deleted' style update.
	// Given the current actor logic, 'SaveComment' is called for new comments.

	commentQuery := `
		INSERT INTO comments (id, content, author_id, post_id, parent_id, karma, upvotes, downvotes, created_at, updated_at)
		VALUES (:id, :content, :author_id, :post_id, :parent_id, :karma, :upvotes, :downvotes, :created_at, :updated_at)
		ON CONFLICT (id) DO UPDATE SET
			content = EXCLUDED.content,
			karma = EXCLUDED.karma,
			upvotes = EXCLUDED.upvotes,
			downvotes = EXCLUDED.downvotes,
			updated_at = EXCLUDED.updated_at
	`
	// Note: We don't update author_id, post_id, parent_id on conflict

	_, err = tx.NamedExecContext(ctx, commentQuery, comment)
	if err != nil {
		tx.Rollback() // Rollback on error
		return utils.NewAppError(utils.ErrDatabase, "failed to save comment", err)
	}

	// If the comment save was successful, increment the post's comment_count
	// We only do this for new comments. The ON CONFLICT clause handles updates to existing comments.
	// A simple way to check if it was an insert vs an update is not straightforward with ON CONFLICT.
	// However, based on current actor logic, SaveComment is primarily for new comments or full state saves.
	// For incrementing count, we assume this call to SaveComment is for a new, non-deleted comment.
	// A more robust system might involve triggers or checking returned rows from insert.

	// Let's assume if `comment.IsDeleted` was a persisted field and true, we wouldn't increment.
	// Since it's not persisted, we increment. This matches the user's report that counts are off.
	updatePostCountQuery := `UPDATE posts SET comment_count = comment_count + 1, updated_at = NOW() WHERE id = $1`
	result, err := tx.ExecContext(ctx, updatePostCountQuery, comment.PostID)
	if err != nil {
		tx.Rollback() // Rollback on error
		log.Printf("Failed to increment comment_count for post %s: %v. Rolling back comment save.", comment.PostID, err)
		return utils.NewAppError(utils.ErrDatabase, "failed to update post comment_count", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		tx.Rollback() // Rollback if the post wasn't found to update its count
		log.Printf("Post %s not found when trying to increment comment_count. Rolling back comment save.", comment.PostID)
		return utils.NewAppError(utils.ErrNotFound, fmt.Sprintf("post %s not found to update comment count", comment.PostID), nil)
	}

	return tx.Commit()
}

// GetComment fetches a single comment by its ID.
func (p *PostgresDB) GetComment(ctx context.Context, id uuid.UUID) (*models.Comment, error) {
	// TODO: Consider adding requestingUserID here as well if individual comment GETs need vote status
	query := `
		SELECT
			c.id, c.content, c.author_id, u.username AS author_username, c.post_id,
			p.subreddit_id, c.parent_id, c.created_at, c.updated_at,
			c.upvotes, c.downvotes, c.karma
		FROM comments c
		JOIN users u ON c.author_id = u.id
		JOIN posts p ON c.post_id = p.id
		WHERE c.id = $1
	`
	var comment models.Comment
	err := p.DB.GetContext(ctx, &comment, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, utils.NewAppError(utils.ErrNotFound, "comment not found", err)
		}
		return nil, utils.NewAppError(utils.ErrDatabase, "failed to query comment by id", err)
	}
	return &comment, nil
}

// GetPostComments fetches all comments for a given post, including the requesting user's vote.
func (p *PostgresDB) GetPostComments(ctx context.Context, postID uuid.UUID, requestingUserID uuid.UUID) ([]*models.Comment, error) {
	// Temporary struct to scan vote_type robustly
	type ScanComment struct {
		models.Comment
		RawVoteType sql.NullString `db:"current_user_vote"`
	}

	query := `
		SELECT
			c.id, c.content, c.author_id, u.username AS author_username, c.post_id,
			p.subreddit_id, c.parent_id, c.created_at, c.updated_at,
			c.upvotes, c.downvotes, c.karma,
			v.vote_type AS current_user_vote
		FROM comments c
		JOIN users u ON c.author_id = u.id
		JOIN posts p ON c.post_id = p.id
		LEFT JOIN votes v ON c.id = v.content_id AND v.content_type = 'comment' AND v.user_id = $2
		WHERE c.post_id = $1
		ORDER BY c.created_at ASC
	`
	var scannedComments []*ScanComment
	err := p.DB.SelectContext(ctx, &scannedComments, query, postID, requestingUserID)
	if err != nil {
		log.Printf("Error querying post comments: %v. Query: %s, PostID: %s, UserID: %s", err, query, postID, requestingUserID)
		return nil, utils.NewAppError(utils.ErrDatabase, "failed to query post comments", err)
	}

	comments := make([]*models.Comment, len(scannedComments))
	for i, sc := range scannedComments {
		comment := sc.Comment // Extract the embedded Comment
		if sc.RawVoteType.Valid {
			rawVote := sc.RawVoteType.String
			if rawVote == "1" || rawVote == "up" { // Handle integer or potential string "up"
				up := "up"
				comment.CurrentUserVote = &up
			} else if rawVote == "-1" || rawVote == "down" { // Handle integer or potential string "down"
				down := "down"
				comment.CurrentUserVote = &down
			}
		}
		// Ensure AuthorUsername is populated (it should be from the JOIN)
		if comment.AuthorUsername == "" && sc.Comment.AuthorUsername != "" { // Redundant check, but safe
			comment.AuthorUsername = sc.Comment.AuthorUsername
		}
		comments[i] = &comment
	}

	return comments, nil
}

// DeleteCommentAndDecrementCount performs a hard delete of a comment and decrements the comment_count on the post.
func (p *PostgresDB) DeleteCommentAndDecrementCount(ctx context.Context, commentID uuid.UUID) error {
	tx, err := p.DB.BeginTxx(ctx, nil)
	if err != nil {
		return utils.NewAppError(utils.ErrDatabase, "failed to begin transaction for delete comment", err)
	}

	var postID uuid.UUID
	// Get the post_id of the comment to be deleted
	getPostIDQuery := `SELECT post_id FROM comments WHERE id = $1`
	err = tx.GetContext(ctx, &postID, getPostIDQuery, commentID)
	if err != nil {
		tx.Rollback()
		if err == sql.ErrNoRows {
			return utils.NewAppError(utils.ErrNotFound, fmt.Sprintf("comment %s not found for deletion", commentID), err)
		}
		return utils.NewAppError(utils.ErrDatabase, "failed to get post_id from comment for deletion", err)
	}

	// Delete the comment
	deleteCommentQuery := `DELETE FROM comments WHERE id = $1`
	result, err := tx.ExecContext(ctx, deleteCommentQuery, commentID)
	if err != nil {
		tx.Rollback()
		return utils.NewAppError(utils.ErrDatabase, "failed to delete comment", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		tx.Rollback() // Should not happen if GetContext above found it, but as a safeguard.
		return utils.NewAppError(utils.ErrNotFound, fmt.Sprintf("comment %s not found during deletion exec, though it was found earlier", commentID), nil)
	}

	// Decrement the post's comment_count
	updatePostCountQuery := `UPDATE posts SET comment_count = GREATEST(0, comment_count - 1), updated_at = NOW() WHERE id = $1`
	postUpdateResult, err := tx.ExecContext(ctx, updatePostCountQuery, postID)
	if err != nil {
		tx.Rollback()
		log.Printf("Failed to decrement comment_count for post %s: %v. Rolling back comment deletion.", postID, err)
		return utils.NewAppError(utils.ErrDatabase, "failed to update post comment_count after deleting comment", err)
	}

	postRowsAffected, _ := postUpdateResult.RowsAffected()
	if postRowsAffected == 0 {
		tx.Rollback()
		log.Printf("Post %s not found when trying to decrement comment_count after comment deletion. Rolling back.", postID)
		// This indicates a data integrity issue if the comment had a post_id for a non-existent post.
		return utils.NewAppError(utils.ErrNotFound, fmt.Sprintf("post %s associated with deleted comment %s not found for count update", postID, commentID), nil)
	}

	return tx.Commit()
}

// GetAllComments fetches all comments (used for initial loading).
func (p *PostgresDB) GetAllComments(ctx context.Context) ([]*models.Comment, error) {
	query := `SELECT id, content, author_id, post_id, parent_id, karma, upvotes, downvotes, created_at, updated_at FROM comments ORDER BY created_at ASC`
	var comments []*models.Comment
	err := p.DB.SelectContext(ctx, &comments, query)
	if err != nil {
		return nil, utils.NewAppError(utils.ErrDatabase, "failed to query all comments", err)
	}
	if comments == nil {
		comments = make([]*models.Comment, 0)
	}
	return comments, nil
}

// --- Message Methods ---

// SaveMessage inserts a new direct message.
func (p *PostgresDB) SaveMessage(ctx context.Context, msg *models.DirectMessage) error {
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	// Note: msg.ReadAt is handled by UpdateMessageStatus

	query := `
		INSERT INTO messages (id, sender_id, receiver_id, content, created_at, read_at)
		VALUES (:id, :sender_id, :receiver_id, :content, :created_at, :read_at)
	`
	_, err := p.DB.NamedExecContext(ctx, query, msg)
	if err != nil {
		return utils.NewAppError(utils.ErrDatabase, "failed to save message", err)
	}
	return nil
}

// GetMessagesByUser fetches all messages sent or received by a user.
func (p *PostgresDB) GetMessagesByUser(ctx context.Context, userID uuid.UUID) ([]*models.DirectMessage, error) {
	query := `
		SELECT id, sender_id, receiver_id, content, created_at, read_at 
		FROM messages 
		WHERE sender_id = $1 OR receiver_id = $1 
		ORDER BY created_at ASC
	`
	var messages []*models.DirectMessage
	err := p.DB.SelectContext(ctx, &messages, query, userID)
	if err != nil {
		return nil, utils.NewAppError(utils.ErrDatabase, "failed to query user messages", err)
	}
	if messages == nil {
		messages = make([]*models.DirectMessage, 0)
	}
	// Set IsRead based on ReadAt for every message
	for _, msg := range messages {
		msg.IsRead = msg.ReadAt != nil
	}
	return messages, nil
}

// UpdateMessageStatus updates the read status of a message.
// Note: The IsDeleted flag from the interface is ignored as it's not in the DB schema.
func (p *PostgresDB) UpdateMessageStatus(ctx context.Context, msgID uuid.UUID, isRead *bool, isDeleted *bool) error {
	if isRead == nil || !*isRead {
		// We only care about marking as read. If isRead is nil or false, do nothing.
		return nil
	}

	// Set read_at to current time if isRead is true
	query := `UPDATE messages SET read_at = NOW() WHERE id = $1 AND read_at IS NULL`
	result, err := p.DB.ExecContext(ctx, query, msgID)
	if err != nil {
		return utils.NewAppError(utils.ErrDatabase, "failed to update message read status", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		// This isn't necessarily an error - the message might not exist or might already be read.
		// Depending on requirements, could return ErrNotFound or just log.
		// log.Printf("Message %s not found or already marked as read during status update", msgID)
	}

	return nil
}

// Implementation of repository methods will go here
// This is just a starting template - you'll need to implement all the repository
// methods that are currently defined in your PostgreSQL implementation
