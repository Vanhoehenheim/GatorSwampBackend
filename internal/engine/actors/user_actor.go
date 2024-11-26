package actors

import (
    "crypto/rand"
    "encoding/base64"
    "github.com/asynkron/protoactor-go/actor"
    "github.com/google/uuid"
    "golang.org/x/crypto/bcrypt"
)

// Message types for UserActor
type (
    RegisterUserMsg struct {
        Username string
        Email    string
        Password string
        Karma int
    }

    UpdateProfileMsg struct {
        UserID      uuid.UUID
        NewUsername string
        NewEmail    string
    }

    UpdateKarmaMsg struct {
        UserID uuid.UUID
        Delta  int // Positive for upvote, negative for downvote
    }

    GetUserProfileMsg struct {
        UserID uuid.UUID
    }

    LoginMsg struct {
        Email    string
        Password string
    }

    LoginResponse struct {
        Success bool
        Token   string
        Error   string
    }
)

// UserState represents the internal state
type UserState struct {
    ID            uuid.UUID
    Username      string
    Email         string
    Karma         int
    Posts         []uuid.UUID
    Comments      []uuid.UUID
    HashedPassword string
    AuthToken     string
}

// UserActor manages user-related operations
type UserActor struct {
    context actor.Context
    state   *UserState
}

func NewUserActor(context actor.Context) *UserActor {
    return &UserActor{
        context: context,
        state:   &UserState{ID: uuid.New()},
    }
}

func hashPassword(password string) (string, error) {
    bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
    return string(bytes), err
}

func generateToken() (string, error) {
    b := make([]byte, 32)
    _, err := rand.Read(b)
    if err != nil {
        return "", err
    }
    return base64.URLEncoding.EncodeToString(b), nil
}

func (a *UserActor) Receive(context actor.Context) {
    switch msg := context.Message().(type) {
    case *RegisterUserMsg:
        // Hash password before storing
        hashedPassword, err := hashPassword(msg.Password)
        if err != nil {
            context.Respond(nil)
            return
        }
        
        a.state.Username = msg.Username
        a.state.Email = msg.Email
        a.state.HashedPassword = hashedPassword
        a.state.Karma = 300
        context.Respond(&UserState{
            ID:       a.state.ID,
            Username: a.state.Username,
            Email:    a.state.Email,
            Karma:    a.state.Karma,

        })

    case *UpdateProfileMsg:
        if a.state.ID == msg.UserID {
            a.state.Username = msg.NewUsername
            a.state.Email = msg.NewEmail
            context.Respond(true)
        } else {
            context.Respond(false)
        }

    case *UpdateKarmaMsg:
        if a.state.ID == msg.UserID {
            a.state.Karma += msg.Delta
            context.Respond(a.state.Karma)
        }

    case *GetUserProfileMsg:
        if a.state.ID == msg.UserID {
            context.Respond(a.state)
        } else {
            context.Respond(nil)
        }

    case *LoginMsg:
        if a.state.Email != msg.Email {
            context.Respond(&LoginResponse{Success: false, Error: "Invalid credentials"})
            return
        }
        
        err := bcrypt.CompareHashAndPassword([]byte(a.state.HashedPassword), []byte(msg.Password))
        if err != nil {
            context.Respond(&LoginResponse{Success: false, Error: "Invalid credentials"})
            return
        }
        
        token, err := generateToken()
        if err != nil {
            context.Respond(&LoginResponse{Success: false, Error: "Authentication error"})
            return
        }
        
        a.state.AuthToken = token
        context.Respond(&LoginResponse{
            Success: true,
            Token:   token,
        })
    }
}