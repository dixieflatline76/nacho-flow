package ui_test

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"blackjack/internal/deck"
	"blackjack/internal/game"
	"blackjack/internal/ui"
)

func TestFormatCard(t *testing.T) {
	c1 := deck.NewCard(deck.Hearts, deck.Ace)
	s1 := ui.FormatCard(c1)
	if !strings.Contains(s1, "A♥") {
		t.Errorf("expected A♥ in formatted card, got %q", s1)
	}

	c2 := deck.NewCard(deck.Spades, deck.Ten)
	s2 := ui.FormatCard(c2)
	if !strings.Contains(s2, "10♠") {
		t.Errorf("expected 10♠ in formatted card, got %q", s2)
	}

	c3 := deck.NewCard(deck.Diamonds, deck.Two)
	s3 := ui.FormatCard(c3)
	if !strings.Contains(s3, "2♦") {
		t.Errorf("expected 2♦ in formatted card, got %q", s3)
	}
}

func TestFormatHand(t *testing.T) {
	h := game.NewHand(25)
	h.AddCard(deck.NewCard(deck.Spades, deck.Ten))
	h.AddCard(deck.NewCard(deck.Hearts, deck.Ace))

	res := ui.FormatHand(h)
	if !strings.Contains(res, "10♠") || !strings.Contains(res, "A♥") {
		t.Errorf("hand string missing cards: %q", res)
	}
	if !strings.Contains(res, "Soft 21") {
		t.Errorf("expected (Soft 21) in hand output, got %q", res)
	}

	// Busted hand
	h2 := game.NewHand(25)
	h2.AddCard(deck.NewCard(deck.Spades, deck.Ten))
	h2.AddCard(deck.NewCard(deck.Hearts, deck.Ten))
	h2.AddCard(deck.NewCard(deck.Clubs, deck.Five))
	h2.IsBusted = true
	res2 := ui.FormatHand(h2)
	if !strings.Contains(res2, "BUST") {
		t.Errorf("expected BUST in output: %q", res2)
	}
}

func TestFormatDealerHand(t *testing.T) {
	d := game.NewHand(0)
	d.AddCard(deck.NewCard(deck.Spades, deck.Ace))
	d.AddCard(deck.NewCard(deck.Hearts, deck.Eight))

	// Face down hides card 0
	fd := ui.FormatDealerHand(d, true)
	if !strings.Contains(fd, "[ ?? ]") {
		t.Errorf("expected [ ?? ] in dealer hand, got %q", fd)
	}
	if !strings.Contains(fd, "8♥") {
		t.Errorf("expected 8♥ in dealer hand, got %q", fd)
	}

	// Face up shows all cards
	fu := ui.FormatDealerHand(d, false)
	if !strings.Contains(fu, "A♠") || !strings.Contains(fu, "8♥") {
		t.Errorf("expected both cards in face up dealer hand, got %q", fu)
	}
}

func TestFormatStatus(t *testing.T) {
	dealer := game.NewHand(0)
	dealer.AddCard(deck.NewCard(deck.Clubs, deck.Ten))
	dealer.AddCard(deck.NewCard(deck.Diamonds, deck.Seven))

	p1 := game.NewHand(50)
	p1.AddCard(deck.NewCard(deck.Hearts, deck.Nine))
	p1.AddCard(deck.NewCard(deck.Spades, deck.Eight))

	status := ui.FormatStatus(1000, []*game.Hand{p1}, dealer, true)
	if !strings.Contains(status, "Bankroll: $1000") {
		t.Errorf("status missing bankroll: %q", status)
	}
	if !strings.Contains(status, "Player:") {
		t.Errorf("status missing player: %q", status)
	}

	// Multiple split hands
	p2 := game.NewHand(50)
	p2.AddCard(deck.NewCard(deck.Hearts, deck.Nine))
	p2.AddCard(deck.NewCard(deck.Spades, deck.Two))
	splitStatus := ui.FormatStatus(950, []*game.Hand{p1, p2}, dealer, false)
	if !strings.Contains(splitStatus, "Hand 1:") || !strings.Contains(splitStatus, "Hand 2:") {
		t.Errorf("split status missing hand numbers: %q", splitStatus)
	}
}

func TestFormatOutcomeAndHint(t *testing.T) {
	winStr := ui.FormatOutcome(game.OutcomeWin, 50)
	if !strings.Contains(winStr, "WIN") || !strings.Contains(winStr, "+50") {
		t.Errorf("expected WIN (+50), got %q", winStr)
	}

	lossStr := ui.FormatOutcome(game.OutcomeLoss, -50)
	if !strings.Contains(lossStr, "LOSS") || !strings.Contains(lossStr, "-50") {
		t.Errorf("expected LOSS (-50), got %q", lossStr)
	}

	hint := ui.FormatHint(game.ActionHit)
	if !strings.Contains(hint, "Hit") {
		t.Errorf("expected Hit in hint: %q", hint)
	}
}

func TestPromptBet(t *testing.T) {
	input := "abc\n5\n100\n"
	reader := bufio.NewReader(strings.NewReader(input))
	var out bytes.Buffer

	bet, ok := ui.PromptBet(reader, &out, 10, 500, 1000)
	if !ok || bet != 100 {
		t.Errorf("expected bet 100, got %d (ok=%v)", bet, ok)
	}

	// Quit test
	qReader := bufio.NewReader(strings.NewReader("q\n"))
	var qOut bytes.Buffer
	_, qOk := ui.PromptBet(qReader, &qOut, 10, 500, 1000)
	if qOk {
		t.Errorf("expected ok=false on quit")
	}
}

func TestPromptAction(t *testing.T) {
	input := "h\n"
	reader := bufio.NewReader(strings.NewReader(input))
	var out bytes.Buffer
	a := ui.PromptAction(reader, &out, true, true)
	if a != game.ActionHit {
		t.Errorf("expected ActionHit, got %v", a)
	}

	inputStand := "stand\n"
	a2 := ui.PromptAction(bufio.NewReader(strings.NewReader(inputStand)), &out, false, false)
	if a2 != game.ActionStand {
		t.Errorf("expected ActionStand, got %v", a2)
	}

	inputDouble := "d\n"
	a3 := ui.PromptAction(bufio.NewReader(strings.NewReader(inputDouble)), &out, true, false)
	if a3 != game.ActionDouble {
		t.Errorf("expected ActionDouble, got %v", a3)
	}

	inputSplit := "l\n"
	a4 := ui.PromptAction(bufio.NewReader(strings.NewReader(inputSplit)), &out, false, true)
	if a4 != game.ActionSplit {
		t.Errorf("expected ActionSplit, got %v", a4)
	}

	inputQuit := "q\n"
	a5 := ui.PromptAction(bufio.NewReader(strings.NewReader(inputQuit)), &out, false, false)
	if int(a5) != -1 {
		t.Errorf("expected -1 on quit, got %v", a5)
	}
}

func TestPromptContinue(t *testing.T) {
	yReader := bufio.NewReader(strings.NewReader("y\n"))
	var out bytes.Buffer
	if !ui.PromptContinue(yReader, &out) {
		t.Errorf("expected true for 'y'")
	}

	nReader := bufio.NewReader(strings.NewReader("n\n"))
	if ui.PromptContinue(nReader, &out) {
		t.Errorf("expected false for 'n'")
	}
}
