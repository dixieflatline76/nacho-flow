package ui

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"blackjack/internal/deck"
	"blackjack/internal/game"
)

// ANSI escape sequences for colored output.
const (
	ANSIReset  = "\033[0m"
	ANSIBold   = "\033[1m"
	ANSIRed    = "\033[31m"
	ANSIGreen  = "\033[32m"
	ANSIYellow = "\033[33m"
	ANSIWhite  = "\033[37m"
	ANSICyan   = "\033[36m"
	ANSIMagenta = "\033[35m"
)

// IsRedSuit returns true for hearts/diamonds (rendered red in ANSI).
func IsRedSuit(s deck.Suit) bool {
	return s == deck.Hearts || s == deck.Diamonds
}

// FormatCard renders a single card in compact ANSI format: [ 10♠ ]
func FormatCard(c deck.Card) string {
	color := ANSIWhite
	if IsRedSuit(c.Suit) {
		color = ANSIRed
	}

	rankStr := c.Rank.String()
	if len(rankStr) == 1 {
		rankStr = " " + rankStr
	}

	return fmt.Sprintf("%s[ %s%s ]%s", color, rankStr, c.Suit.String(), ANSIReset)
}

// FormatHand renders a player hand with total: "[ 10♠ ] [  A♥ ] (Soft 21)"
func FormatHand(h *game.Hand) string {
	var sb strings.Builder
	for i, c := range h.Cards {
		if i > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(FormatCard(c))
	}

	_, soft, isSoft := h.Total()
	value := h.BestValue()
	label := fmt.Sprintf("(%d)", value)
	if isSoft {
		label = fmt.Sprintf("(Soft %d)", soft)
	}
	if h.IsBusted {
		label = fmt.Sprintf("(BUST %d)", value)
	}

	sb.WriteString(" ")
	sb.WriteString(ANSICyan)
	sb.WriteString(label)
	sb.WriteString(ANSIReset)

	return sb.String()
}

// FormatDealerHand renders dealer hand (hides hole card when faceDown).
func FormatDealerHand(h *game.Hand, faceDown bool) string {
	var sb strings.Builder
	for i, c := range h.Cards {
		if i > 0 {
			sb.WriteString(" ")
		}
		if faceDown && i == 0 {
			sb.WriteString("[ ?? ]")
			continue
		}
		sb.WriteString(FormatCard(c))
	}
	return sb.String()
}

// FormatStatus renders the game state summary line (dealer upcard, hands, bankroll).
func FormatStatus(bankroll int, hands []*game.Hand, dealer *game.Hand, dealerFaceDown bool) string {
	var sb strings.Builder
	sb.WriteString(ANSIBold)
	sb.WriteString(fmt.Sprintf("Bankroll: $%d", bankroll))
	sb.WriteString(ANSIReset)
	sb.WriteString(" | ")
	sb.WriteString("Dealer: ")
	sb.WriteString(FormatDealerHand(dealer, dealerFaceDown))
	for i, h := range hands {
		sb.WriteString(" | ")
		if len(hands) > 1 {
			sb.WriteString(fmt.Sprintf("Hand %d: ", i+1))
		} else {
			sb.WriteString("Player: ")
		}
		sb.WriteString(FormatHand(h))
		sb.WriteString(fmt.Sprintf(" (bet $%d)", h.Bet))
	}
	return sb.String()
}

// FormatOutcome renders the result of a round with color.
func FormatOutcome(o game.Outcome, payout int) string {
	var color string
	switch o {
	case game.OutcomeWin, game.OutcomeBlackjack:
		color = ANSIGreen
	case game.OutcomeLoss, game.OutcomeBust, game.OutcomeSurrender:
		color = ANSIRed
	default:
		color = ANSIYellow
	}
	payoutStr := fmt.Sprintf("%+d", payout)
	return fmt.Sprintf("%s%s (%s)%s", color, o.String(), payoutStr, ANSIReset)
}

// FormatHint renders the basic strategy hint.
func FormatHint(a game.Action) string {
	return fmt.Sprintf("%s[Hint: %s]%s", ANSIMagenta, a.String(), ANSIReset)
}

// Println writes a line to the output.
func Println(w io.Writer, msg string) {
	fmt.Fprintln(w, msg)
}

// PromptAction displays available options and reads the player's action choice.
// Returns the chosen game.Action, or -1 if user chose to quit.
func PromptAction(reader *bufio.Reader, w io.Writer, canDouble, canSplit bool) game.Action {
	options := "Actions: (h)it  (s)tand"
	if canDouble {
		options += "  (d)ouble"
	}
	if canSplit {
		options += "  sp(l)it"
	}
	options += "  (q)uit"

	for {
		fmt.Fprintf(w, "%s > ", options)
		line, err := reader.ReadString('\n')
		if err != nil {
			return game.Action(-1)
		}
		line = strings.TrimSpace(strings.ToLower(line))
		switch line {
		case "h", "hit":
			return game.ActionHit
		case "s", "stand":
			return game.ActionStand
		case "d", "double":
			if canDouble {
				return game.ActionDouble
			}
			fmt.Fprintln(w, "Cannot double now.")
		case "l", "split", "p":
			if canSplit {
				return game.ActionSplit
			}
			fmt.Fprintln(w, "Cannot split now.")
		case "q", "quit", "exit":
			return game.Action(-1)
		default:
			fmt.Fprintln(w, "Invalid input. Try again.")
		}
	}
}

// PromptBet reads a bet amount from user input, clamped to min/max rules and bankroll.
func PromptBet(reader *bufio.Reader, w io.Writer, minBet, maxBet, bankroll int) (int, bool) {
	for {
		fmt.Fprintf(w, "Place your bet ($%d-$%d, bankroll $%d) or (q)uit > ", minBet, minInt(maxBet, bankroll), bankroll)
		line, err := reader.ReadString('\n')
		if err != nil {
			return 0, false
		}
		line = strings.TrimSpace(strings.ToLower(line))
		if line == "q" || line == "quit" || line == "exit" {
			return 0, false
		}
		bet, err := strconv.Atoi(line)
		if err != nil {
			fmt.Fprintln(w, "Please enter a valid number.")
			continue
		}
		if bet < minBet || bet > maxBet {
			fmt.Fprintf(w, "Bet must be between $%d and $%d.\n", minBet, maxBet)
			continue
		}
		if bet > bankroll {
			fmt.Fprintln(w, "Insufficient bankroll.")
			continue
		}
		return bet, true
	}
}

// PromptContinue asks whether to play another round.
func PromptContinue(reader *bufio.Reader, w io.Writer) bool {
	fmt.Fprintf(w, "Play another round? (y/n) > ")
	line, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes" || line == ""
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
