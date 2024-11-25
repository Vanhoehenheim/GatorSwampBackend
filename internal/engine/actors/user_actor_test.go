package actors

import (
    "testing"
    "time"
    "github.com/asynkron/protoactor-go/actor"
    "github.com/stretchr/testify/assert"
)

func TestUserAuthentication(t *testing.T) {
    // Create the actor system
    system := actor.NewActorSystem()
    
    // Create a new user actor
    props := actor.PropsFromProducer(func() actor.Actor {
        return NewUserActor(nil)
    })
    
    pid := system.Root.Spawn(props)
    
    // Step 1: Register a new user
    regFuture := system.Root.RequestFuture(pid, &RegisterUserMsg{
        Username: "testuser",
        Email:    "test@example.com",
        Password: "password123",
    }, 5*time.Second)  // Increased timeout
    
    regResult, err := regFuture.Result()
    if err != nil {
        t.Fatalf("Registration failed: %v", err)
    }
    
    userState, ok := regResult.(*UserState)
    if !ok {
        t.Fatal("Failed to get user state from registration")
    }
    assert.Equal(t, "testuser", userState.Username)
    
    // Step 2: Try logging in
    loginFuture := system.Root.RequestFuture(pid, &LoginMsg{
        Email:    "test@example.com",
        Password: "password123",
    }, 5*time.Second)  // Increased timeout
    
    loginResult, err := loginFuture.Result()
    if err != nil {
        t.Fatalf("Login failed: %v", err)
    }
    
    loginResponse, ok := loginResult.(*LoginResponse)
    if !ok {
        t.Fatal("Failed to get login response")
    }
    
    // Verify login success
    assert.True(t, loginResponse.Success)
    assert.NotEmpty(t, loginResponse.Token)
    
    // Step 3: Test invalid login
    badLoginFuture := system.Root.RequestFuture(pid, &LoginMsg{
        Email:    "test@example.com",
        Password: "wrongpassword",
    }, 5*time.Second)
    
    badLoginResult, err := badLoginFuture.Result()
    if err != nil {
        t.Fatalf("Bad login request failed: %v", err)
    }
    
    badLoginResponse, ok := badLoginResult.(*LoginResponse)
    if !ok {
        t.Fatal("Failed to get bad login response")
    }
    
    assert.False(t, badLoginResponse.Success)
    assert.Equal(t, "Invalid credentials", badLoginResponse.Error)
}