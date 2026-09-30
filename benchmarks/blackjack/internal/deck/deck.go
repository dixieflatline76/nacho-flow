package deck

import (
	"errors"
	"fmt"
	"math/rand"
	"time"
)

// Suit represents a playing card suit.
type Suit int

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

// Rank represents the rank/face value of a playing card.
type Rank int

const (
	Two Rank = iota + 2
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

// Card represents a single playing card.
type Card struct {
	Suit  Suit
	Rank  Rank
	Value int // Base blackjack value: 2-10, Face cards=10, Ace=11
}

// NewCard constructs a Card and computes its standard Blackjack value.
func NewCard(suit Suit, rank Rank) Card {
	var val int
	switch rank {
	case Two, Three, Four, Five, Six, Seven, Eight, Nine, Ten:
		val = int(rank)
	case Jack, Queen, King:
		val = 10
	case Ace:
		val = 11
	default:
		val = 0
	}
	return Card{
		Suit:  suit,
		Rank:  rank,
		Value: val,
	}
}

// String returns a compact representation like "10♠" or "A♥".
func (c Card) String() string {
	return fmt.Sprintf("%s%s", c.Rank.String(), c.Suit.String())
}

// Shoe represents a multi-deck shoe (1-8 standard 52-card decks).
type Shoe struct {
	cards          []Card
	cursor         int
	numDecks       int
	penetration    float64
	cutCardIndex   int
	needsReshuffle bool
	rng            *rand.Rand
}

var (
	ErrShoeEmpty    = errors.New("shoe is empty")
	ErrInvalidDecks = errors.New("number of decks must be between 1 and 8")
)

// NewShoe initializes a Shoe with the specified number of decks (1-8) and penetration percentage (0.5 to 0.9).
func NewShoe(numDecks int, penetration float64) (*Shoe, error) {
	if numDecks < 1 || numDecks > 8 {
		return nil, ErrInvalidDecks
	}
	if penetration <= 0 || penetration >= 1.0 {
		penetration = 0.75
	}

	src := rand.NewSource(time.Now().UnixNano())
	shoe := &Shoe{
		numDecks:    numDecks,
		penetration: penetration,
		rng:         rand.New(src),
	}
	shoe.Reset()
	return shoe, nil
}

// NewShoeWithRNG creates a Shoe with a custom *rand.Rand, useful for deterministic testing.
func NewShoeWithRNG(numDecks int, penetration float64, rng *rand.Rand) (*Shoe, error) {
	if numDecks < 1 || numDecks > 8 {
		return nil, ErrInvalidDecks
	}
	if penetration <= 0 || penetration >= 1.0 {
		penetration = 0.75
	}
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	shoe := &Shoe{
		numDecks:    numDecks,
		penetration: penetration,
		rng:         rng,
	}
	shoe.Reset()
	return shoe, nil
}

// Reset repopulates the shoe with standard decks and shuffles using Fisher-Yates.
func (s *Shoe) Reset() {
	suits := []Suit{Spades, Hearts, Diamonds, Clubs}
	ranks := []Rank{Two, Three, Four, Five, Six, Seven, Eight, Nine, Ten, Jack, Queen, King, Ace}

	totalCards := s.numDecks * 52
	s.cards = make([]Card, 0, totalCards)

	for d := 0; d < s.numDecks; d++ {
		for _, st := range suits {
			for _, rk := range ranks {
				s.cards = append(s.cards, NewCard(st, rk))
			}
		}
	}

	s.Shuffle()
	s.cursor = 0
	s.cutCardIndex = int(float64(totalCards) * s.penetration)
	s.needsReshuffle = false
}

// Shuffle applies the in-place Fisher-Yates algorithm across all cards in the shoe.
func (s *Shoe) Shuffle() {
	n := len(s.cards)
	for i := n - 1; i > 0; i-- {
		j := s.rng.Intn(i + 1)
		s.cards[i], s.cards[j] = s.cards[j], s.cards[i]
	}
}

// Draw pulls the next card from the shoe and updates penetration trigger state.
func (s *Shoe) Draw() (Card, error) {
	if s.cursor >= len(s.cards) {
		return Card{}, ErrShoeEmpty
	}

	card := s.cards[s.cursor]
	s.cursor++

	if s.cursor >= s.cutCardIndex {
		s.needsReshuffle = true
	}

	return card, nil
}

// NeedsReshuffle reports whether the cut-card penetration threshold has been passed.
func (s *Shoe) NeedsReshuffle() bool {
	return s.needsReshuffle
}

// Remaining returns how many cards are left in the shoe before it is exhausted.
func (s *Shoe) Remaining() int {
	return len(s.cards) - s.cursor
}

// TotalCards returns the initial card capacity of the shoe.
func (s *Shoe) TotalCards() int {
	return len(s.cards)
}

// CutCardIndex returns the index at which the cut card is placed.
func (s *Shoe) CutCardIndex() int {
	return s.cutCardIndex
}
