package cards

import (
	"errors"
	"fmt"
	"math/rand"
	"time"
)

type Suit string

const (
	Hearts   Suit = "♥"
	Diamonds Suit = "♦"
	Clubs    Suit = "♣"
	Spades   Suit = "♠"
)

type Rank string

const (
	Ace   Rank = "A"
	Two   Rank = "2"
	Three Rank = "3"
	Four  Rank = "4"
	Five  Rank = "5"
	Six   Rank = "6"
	Seven Rank = "7"
	Eight Rank = "8"
	Nine  Rank = "9"
	Ten   Rank = "10"
	Jack  Rank = "J"
	Queen Rank = "Q"
	King  Rank = "K"
)

// Card represents a playing card with its rank, suit, and value.
type Card struct {
	Rank  Rank
	Suit  Suit
	Value int
}

// String returns a string representation of the card, e.g., [A♥].
func (c Card) String() string {
	return fmt.Sprintf("[%s%s]", c.Rank, c.Suit)
}

// CardDefinition is a helper struct to initialize the deck.
type CardDefinition struct {
	Rank  Rank
	Suit  Suit
	Value int // 2-10, J/Q/K = 10, Ace = 11
}

// AllCards contains all 52 unique cards.
var AllCards = []CardDefinition{
	{Ace, Hearts, 11}, {Two, Hearts, 2}, {Three, Hearts, 3}, {Four, Hearts, 4}, {Five, Hearts, 5},
	{Six, Hearts, 6}, {Seven, Hearts, 7}, {Eight, Hearts, 8}, {Nine, Hearts, 9}, {Ten, Hearts, 10},
	{Jack, Hearts, 10}, {Queen, Hearts, 10}, {King, Hearts, 10},

	{Ace, Diamonds, 11}, {Two, Diamonds, 2}, {Three, Diamonds, 3}, {Four, Diamonds, 4}, {Five, Diamonds, 5},
	{Six, Diamonds, 6}, {Seven, Diamonds, 7}, {Eight, Diamonds, 8}, {Nine, Diamonds, 9}, {Ten, Diamonds, 10},
	{Jack, Diamonds, 10}, {Queen, Diamonds, 10}, {King, Diamonds, 10},

	{Ace, Clubs, 11}, {Two, Clubs, 2}, {Three, Clubs, 3}, {Four, Clubs, 4}, {Five, Clubs, 5},
	{Six, Clubs, 6}, {Seven, Clubs, 7}, {Eight, Clubs, 8}, {Nine, Clubs, 9}, {Ten, Clubs, 10},
	{Jack, Clubs, 10}, {Queen, Clubs, 10}, {King, Clubs, 10},

	{Ace, Spades, 11}, {Two, Spades, 2}, {Three, Spades, 3}, {Four, Spades, 4}, {Five, Spades, 5},
	{Six, Spades, 6}, {Seven, Spades, 7}, {Eight, Spades, 8}, {Nine, Spades, 9}, {Ten, Spades, 10},
	{Jack, Spades, 10}, {Queen, Spades, 10}, {King, Spades, 10},
}

// Deck represents a collection of cards.
type Deck struct {
	Cards []Card
}

// NewDeck creates a new standard 52-card deck.
func NewDeck() *Deck {
	cards := make([]Card, len(AllCards))
	for i, def := range AllCards {
		cards[i] = Card{Rank: def.Rank, Suit: def.Suit, Value: def.Value}
	}
	return &Deck{Cards: cards}
}

// Shuffle randomizes the order of cards in the deck using a Fisher-Yates shuffle.
func (d *Deck) Shuffle() {
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(d.Cards), func(i, j int) {
		d.Cards[i], d.Cards[j] = d.Cards[j], d.Cards[i]
	})
}

// Shoe represents a container holding multiple decks of cards.
type Shoe struct {
	Decks      []*Deck
	Cards      []Card // Flattened cards from all decks
	CurrentPos int    // Current position in the shoe
}

// NewShoe creates a new shoe with the specified number of decks.
func NewShoe(numDecks int) *Shoe {
	if numDecks <= 0 {
		numDecks = 1
	}

	shoe := &Shoe{}
	for i := 0; i < numDecks; i++ {
		deck := NewDeck()
		shoe.Decks = append(shoe.Decks, deck)
	}

	// Flatten all cards into a single slice
	totalCards := len(AllCards) * numDecks
	shoe.Cards = make([]Card, totalCards)
	index := 0
	for _, deck := range shoe.Decks {
		for _, card := range deck.Cards {
			shoe.Cards[index] = card
			index++
		}
	}

	return shoe
}

// Shuffle shuffles all cards in the shoe.
func (s *Shoe) Shuffle() {
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(s.Cards), func(i, j int) {
		s.Cards[i], s.Cards[j] = s.Cards[j], s.Cards[i]
	})
	s.CurrentPos = 0
}

// DrawCard removes and returns the top card from the shoe.
func (s *Shoe) DrawCard() (Card, error) {
	if s.CurrentPos >= len(s.Cards) {
		return Card{}, errors.New("no cards left in shoe")
	}
	card := s.Cards[s.CurrentPos]
	s.CurrentPos++
	return card, nil
}

// RemainingCards returns the number of cards left in the shoe.
func (s *Shoe) RemainingCards() int {
	return len(s.Cards) - s.CurrentPos
}

// PenetrationPercentage returns how much of the shoe has been dealt (0.0 to 1.0).
func (s *Shoe) PenetrationPercentage() float64 {
	total := len(s.Cards)
	remaining := s.RemainingCards()
	return float64(total-remaining) / float64(total)
}
