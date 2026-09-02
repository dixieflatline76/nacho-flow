package cards

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// Suit represents a playing card suit.
type Suit int8

const (
	Spades Suit = iota
	Hearts
	Diamonds
	Clubs
)

func (s Suit) String() string {
	switch s {
	case Spades:
		return "♠"
	case Hearts:
		return "♥"
	case Diamonds:
		return "♦"
	case Clubs:
		return "♣"
	default:
		return "?"
	}
}

// Rank represents the face value of a card.
type Rank int8

const (
	Two Rank = 2 + iota
	Three
	Four
	Five
	Six
	Seven
	Eight
	Nine
	Ten
	Jack
	Queen
	King
	Ace
)

func (r Rank) String() string {
	switch r {
	case Two, Three, Four, Five, Six, Seven, Eight, Nine, Ten:
		return fmt.Sprintf("%d", int(r))
	case Jack:
		return "J"
	case Queen:
		return "Q"
	case King:
		return "K"
	case Ace:
		return "A"
	default:
		return "?"
	}
}

// BlackjackValue returns the standard Blackjack point value (Ace is 11 by default).
func (r Rank) BlackjackValue() int {
	switch r {
	case Two, Three, Four, Five, Six, Seven, Eight, Nine:
		return int(r)
	case Ten, Jack, Queen, King:
		return 10
	case Ace:
		return 11
	default:
		return 0
	}
}

// Card represents a single playing card.
type Card struct {
	Rank Rank
	Suit Suit
}

func NewCard(r Rank, s Suit) Card {
	return Card{Rank: r, Suit: s}
}

func (c Card) String() string {
	return fmt.Sprintf("%s%s", c.Rank.String(), c.Suit.String())
}

// IsTenValue returns true if card has blackjack value of 10 (10, J, Q, K).
func (c Card) IsTenValue() bool {
	return c.Rank.BlackjackValue() == 10
}

// IsAce returns true if card is an Ace.
func (c Card) IsAce() bool {
	return c.Rank == Ace
}

// Shoe represents a multi-deck dealing shoe.
type Shoe struct {
	mu           sync.Mutex
	cards        []Card
	dealtCards   []Card
	totalDecks   int
	cutCardIndex int
	rng          *rand.Rand
	onDeal       func(Card)
}

// ShoeOption defines functional options for Shoe creation.
type ShoeOption func(*Shoe)

// WithCutCardPenetration sets the shoe cut card penetration percentage (e.g. 0.75 = 75%).
func WithCutCardPenetration(penetration float64) ShoeOption {
	return func(s *Shoe) {
		if penetration <= 0 || penetration >= 1.0 {
			penetration = 0.75
		}
		total := s.totalDecks * 52
		s.cutCardIndex = int(float64(total) * penetration)
	}
}

// WithCustomRNG sets a custom random generator (useful for reproducible tests).
func WithCustomRNG(rng *rand.Rand) ShoeOption {
	return func(s *Shoe) {
		s.rng = rng
	}
}

// WithDealListener registers a callback invoked every time a card is dealt.
func WithDealListener(callback func(Card)) ShoeOption {
	return func(s *Shoe) {
		s.onDeal = callback
	}
}

// NewShoe initializes and shuffles a multi-deck shoe (1 to 8 decks).
func NewShoe(numDecks int, opts ...ShoeOption) *Shoe {
	if numDecks < 1 {
		numDecks = 1
	}
	if numDecks > 8 {
		numDecks = 8
	}

	s := &Shoe{
		totalDecks: numDecks,
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	// Default cut card index: 75%
	s.cutCardIndex = int(float64(numDecks*52) * 0.75)

	for _, opt := range opts {
		opt(s)
	}

	s.ResetAndShuffle()
	return s
}

// ResetAndShuffle refills the shoe with full decks and performs a Fisher-Yates shuffle.
func (s *Shoe) ResetAndShuffle() {
	s.mu.Lock()
	defer s.mu.Unlock()

	totalCards := s.totalDecks * 52
	s.cards = make([]Card, 0, totalCards)
	s.dealtCards = make([]Card, 0, totalCards)

	allSuits := []Suit{Spades, Hearts, Diamonds, Clubs}
	allRanks := []Rank{Two, Three, Four, Five, Six, Seven, Eight, Nine, Ten, Jack, Queen, King, Ace}

	for d := 0; d < s.totalDecks; d++ {
		for _, suit := range allSuits {
			for _, rank := range allRanks {
				s.cards = append(s.cards, Card{Rank: rank, Suit: suit})
			}
		}
	}

	// Fisher-Yates shuffle
	for i := len(s.cards) - 1; i > 0; i-- {
		j := s.rng.Intn(i + 1)
		s.cards[i], s.cards[j] = s.cards[j], s.cards[i]
	}
}

// Deal pops the top card from the shoe. If empty, it automatically resets & shuffles.
func (s *Shoe) Deal() Card {
	s.mu.Lock()
	if len(s.cards) == 0 {
		s.mu.Unlock()
		s.ResetAndShuffle()
		s.mu.Lock()
	}

	idx := len(s.cards) - 1
	card := s.cards[idx]
	s.cards = s.cards[:idx]
	s.dealtCards = append(s.dealtCards, card)

	cb := s.onDeal
	s.mu.Unlock()

	if cb != nil {
		cb(card)
	}
	return card
}

// NeedsShuffle returns true if dealt cards count has passed the cut card index.
func (s *Shoe) NeedsShuffle() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.dealtCards) >= s.cutCardIndex
}

// RemainingCards returns the number of un-dealt cards in the shoe.
func (s *Shoe) RemainingCards() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.cards)
}

// DealtCount returns the number of cards dealt so far.
func (s *Shoe) DealtCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.dealtCards)
}

// TotalDecks returns the configured deck count.
func (s *Shoe) TotalDecks() int {
	return s.totalDecks
}

// RemainingDecks returns remaining decks estimated as a float (cards / 52.0).
func (s *Shoe) RemainingDecks() float64 {
	rem := float64(s.RemainingCards()) / 52.0
	if rem < 0.5 {
		return 0.5 // Bound minimum remaining decks to prevent division by zero in card counting
	}
	return rem
}

// Penetration returns the percentage of the shoe dealt (0.0 to 1.0).
func (s *Shoe) Penetration() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	total := s.totalDecks * 52
	if total == 0 {
		return 0
	}
	return float64(len(s.dealtCards)) / float64(total)
}
