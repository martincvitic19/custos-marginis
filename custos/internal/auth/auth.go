// Package auth is custosd's login: one admin password (no default; set on
// first use), stored as an argon2id hash, in-memory sessions with a CSRF
// token each, and a login rate limit per client address.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/martincvitic19/custos-marginis/custos/internal/config"
)

// argon2id parameters (RFC 9106 second recommendation, memory reduced for a
// 1 GB router: 64 MiB, 3 passes, 2 lanes, ~0.5 s on the Cortex-A53).
const (
	argTime    = 3
	argMemory  = 64 * 1024
	argThreads = 2
	argKeyLen  = 32
)

// MinPasswordLen is enforced on setup and change.
const MinPasswordLen = 10

// Errors.
var (
	ErrNotSet      = errors.New("password not set")
	ErrAlreadySet  = errors.New("password already set")
	ErrBadPassword = errors.New("wrong password")
	ErrTooMany     = errors.New("too many failed logins")
	ErrWeak        = fmt.Errorf("password must have at least %d characters", MinPasswordLen)
)

type stored struct {
	Hash    string    `json:"hash"`
	Created time.Time `json:"created"`
}

// Session is one login.
type Session struct {
	Token   string
	CSRF    string
	Expires time.Time
}

// Store holds the hash file path, sessions and failure counters.
type Store struct {
	path     string
	mu       sync.Mutex
	sessions map[string]*Session
	fails    map[string][]time.Time
	TTL      time.Duration
	MaxFails int
	Window   time.Duration
	now      func() time.Time
}

// New returns a store backed by path (e.g. /data/custosd/auth.json).
func New(path string) *Store {
	return &Store{path: path, sessions: map[string]*Session{}, fails: map[string][]time.Time{},
		TTL: 12 * time.Hour, MaxFails: 5, Window: 15 * time.Minute, now: time.Now}
}

// IsSet reports whether a password exists.
func (s *Store) IsSet() bool {
	_, err := os.Stat(s.path)
	return err == nil
}

func random(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Hash returns an encoded argon2id hash ($argon2id$v=19$m=..,t=..,p=..$salt$key).
func Hash(pw string) string {
	salt := make([]byte, 16)
	rand.Read(salt)
	key := argon2.IDKey([]byte(pw), salt, argTime, argMemory, argThreads, argKeyLen)
	enc := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argMemory, argTime, argThreads, enc(salt), enc(key))
}

// Verify checks a password against an encoded hash in constant time.
func Verify(pw, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[4])
	key, err2 := base64.RawStdEncoding.DecodeString(parts[5])
	if err1 != nil || err2 != nil {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, t, m, p, uint32(len(key)))
	return subtle.ConstantTimeCompare(got, key) == 1
}

// Setup sets the first password. It fails once a password exists.
func (s *Store) Setup(pw string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.IsSet() {
		return ErrAlreadySet
	}
	return s.write(pw)
}

// Change replaces the password (old one required) and ends all sessions.
func (s *Store) Change(old, pw string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, err := s.read()
	if err != nil {
		return err
	}
	if !Verify(old, h) {
		return ErrBadPassword
	}
	if err := s.write(pw); err != nil {
		return err
	}
	s.sessions = map[string]*Session{}
	return nil
}

func (s *Store) write(pw string) error {
	if len([]rune(pw)) < MinPasswordLen {
		return ErrWeak
	}
	b, _ := json.Marshal(stored{Hash: Hash(pw), Created: s.now()})
	return config.WriteFileAtomic(s.path, b, 0600)
}

func (s *Store) read() (string, error) {
	b, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return "", ErrNotSet
	}
	if err != nil {
		return "", err
	}
	var st stored
	if err := json.Unmarshal(b, &st); err != nil {
		return "", err
	}
	return st.Hash, nil
}

// Blocked reports whether a client address hit the failure limit.
func (s *Store) Blocked(client string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.recentFails(client)) >= s.MaxFails
}

func (s *Store) recentFails(client string) []time.Time {
	cut := s.now().Add(-s.Window)
	var keep []time.Time
	for _, t := range s.fails[client] {
		if t.After(cut) {
			keep = append(keep, t)
		}
	}
	s.fails[client] = keep
	return keep
}

// Login checks the password and returns a new session. After MaxFails
// failures in Window it returns ErrTooMany without checking.
func (s *Store) Login(client, pw string) (*Session, error) {
	s.mu.Lock()
	if len(s.recentFails(client)) >= s.MaxFails {
		s.mu.Unlock()
		return nil, ErrTooMany
	}
	s.mu.Unlock()
	h, err := s.read()
	if err != nil {
		return nil, err
	}
	ok := Verify(pw, h) // slow on purpose; not under the lock
	s.mu.Lock()
	defer s.mu.Unlock()
	if !ok {
		s.fails[client] = append(s.fails[client], s.now())
		return nil, ErrBadPassword
	}
	delete(s.fails, client)
	sess := &Session{Token: random(32), CSRF: random(24), Expires: s.now().Add(s.TTL)}
	s.sessions[sess.Token] = sess
	return sess, nil
}

// Lookup returns the session for a token, if valid.
func (s *Store) Lookup(token string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess := s.sessions[token]
	if sess == nil {
		return nil
	}
	if s.now().After(sess.Expires) {
		delete(s.sessions, token)
		return nil
	}
	return sess
}

// Logout ends a session.
func (s *Store) Logout(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}
