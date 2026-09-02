package ui

import (
	"blackjack/pkg/cards"
	"blackjack/pkg/engine"
	"blackjack/pkg/oracle"
	"fmt"
	"strings"
)

// ANSI color codes for terminal rendering.
const (
	Reset   = "\033[0m"
	Bold    = "\033[1m"
	Dim     = "\033[2m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"
	BgBlack = "\033[40m"
)

// CardRenderer renders ASCII playing cards.
type CardRenderer struct {
	UseColors bool
}

// NewCardRenderer returns a new CardRenderer.
func NewCardRenderer(useColors bool) *CardRenderer {
	return &CardRenderer{UseColors: useColors}
}

// RenderCard returns the 5-line ASCII art representation of a single card.
// Example:
// ┌─────────┐
// │ A       │
// │    ♠    │
// │       A │
// └─────────┘
func (r *CardRenderer) RenderCard(c cards.Card, hidden bool) []string {
	if hidden {
		return []string{
			"┌─────────┐",
			"│░░░░░░░░░│",
			"│░░░ 🂠 ░░░│",
			"│░░░░░░░░░│",
			"└─────────┘",
		}
	}

	rankStr := c.Rank.String()
	suitStr := c.Suit.String()

	leftRank := fmt.Sprintf("%-2s", rankStr)
	rightRank := fmt.Sprintf("%2s", rankStr)

	suitColor := White
	if r.UseColors {
		if c.Suit == cards.Hearts || c.Suit == cards.Diamonds {
			suitColor = Red
		} else {
			suitColor = Cyan
		}
	}

	coloredSuit := suitStr
	if r.UseColors {
		coloredSuit = fmt.Sprintf("%s%s%s", suitColor, suitStr, Reset)
	}

	line0 := "┌─────────┐"
	line1 := fmt.Sprintf("│ %s      │", leftRank)
	line2 := fmt.Sprintf("│    %s    │", coloredSuit)
	line3 := fmt.Sprintf("│      %s │", rightRank)
	line4 := "└─────────┘"

	return []string{line0, line1, line2, line3, line4}
}

// RenderHandMultiLine renders multiple cards horizontally side-by-side.
func (r *CardRenderer) RenderHandMultiLine(handCards []cards.Card, hideHoleCard bool) string {
	if len(handCards) == 0 {
		return "[Empty Hand]"
	}

	cardLines := make([][]string, len(handCards))
	for i, c := range handCards {
		isHidden := hideHoleCard && i == 1
		cardLines[i] = r.RenderCard(c, isHidden)
	}

	var sb strings.Builder
	for lineIdx := 0; lineIdx < 5; lineIdx++ {
		for cardIdx := 0; cardIdx < len(handCards); cardIdx++ {
			sb.WriteString(cardLines[cardIdx][lineIdx])
			sb.WriteString("  ")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// FormatOracleHUD formats the card counting and real-time basic strategy recommendations.
func FormatOracleHUD(counter *oracle.CardCounter, advisor *oracle.StrategyAdvisor, activeHand *engine.Hand, dealerUpcard cards.Card) string {
	var sb strings.Builder

	rc := counter.RunningCount()
	tc := counter.TrueCount()
	pen := counter.ShoePenetration() * 100.0
	remDecks := counter.RemainingDecks()
	betMult := counter.RecommendedBetMultiplier()

	sb.WriteString(fmt.Sprintf("%s=== 🔮 CARD COUNTING ORACLE HUD ===%s\n", Bold+Cyan, Reset))
	sb.WriteString(fmt.Sprintf("  Running Count (RC): %+d\n", rc))
	sb.WriteString(fmt.Sprintf("  True Count (TC): %+.2f   | Remaining Decks: %.2f\n", tc, remDecks))
	sb.WriteString(fmt.Sprintf("  Shoe Penetration: %5.1f%% | Kelly Bet Sizing: %dx Base Bet\n", pen, betMult))

	if activeHand != nil {
		rec := advisor.Advise(activeHand, dealerUpcard, tc)
		actionColor := Green
		switch rec.Action {
		case engine.ActionHit:
			actionColor = Yellow
		case engine.ActionDouble:
			actionColor = Cyan
		case engine.ActionSplit:
			actionColor = Magenta
		case engine.ActionSurrender:
			actionColor = Red
		case engine.ActionStand:
			actionColor = Blue
		}
		sb.WriteString(fmt.Sprintf("  Advisor Recommendation: %s%s%s (%s)\n", Bold+actionColor, rec.Action.String(), Reset, rec.Description))
	}
	sb.WriteString(fmt.Sprintf("%s=================================%s\n", Dim+Cyan, Reset))

	return sb.String()
}

// FormatTableState generates a full ASCII snapshot of the current blackjack table.
func FormatTableState(
	r *CardRenderer,
	game *engine.GameEngine,
	counter *oracle.CardCounter,
	advisor *oracle.StrategyAdvisor,
	showOracle bool,
) string {
	var sb strings.Builder

	sb.WriteString("\n" + strings.Repeat("═", 60) + "\n")
	sb.WriteString(fmt.Sprintf("  TABLE RULES: %s (%d Decks, %s, %s)\n",
		game.Rules.Name, game.Rules.Decks,
		map[bool]string{true: "H17", false: "S17"}[game.Rules.DealerHitsSoft17],
		game.Rules.BlackjackPayout.String()))
	sb.WriteString(strings.Repeat("═", 60) + "\n\n")

	// 1. Dealer Hand
	hideDealerHole := game.HasHoleCard && (game.State == engine.StatePlayerTurn || game.State == engine.StateInsuranceOffered)
	dealerDisplayCards := game.DealerHand.Cards
	if hideDealerHole && game.HasHoleCard {
		dealerDisplayCards = []cards.Card{game.DealerHand.Cards[0], game.DealerHoleCard}
	}

	dealerScoreStr := "?"
	if !hideDealerHole {
		tot, isSoft := game.DealerHand.Score()
		softStr := ""
		if isSoft && tot < 21 {
			softStr = " (soft)"
		}
		dealerScoreStr = fmt.Sprintf("%d%s - %s", tot, softStr, game.DealerHand.Status.String())
	}

	sb.WriteString(fmt.Sprintf("%s🏦 DEALER HAND%s (Score: %s)\n", Bold+Yellow, Reset, dealerScoreStr))
	sb.WriteString(r.RenderHandMultiLine(dealerDisplayCards, hideDealerHole))
	sb.WriteString("\n")

	// 2. Player Hands
	sb.WriteString(fmt.Sprintf("%s👤 PLAYER HANDS%s\n", Bold+Green, Reset))
	for i, hand := range game.PlayerHands {
		prefix := "  "
		if i == game.ActiveHandIdx && game.State == engine.StatePlayerTurn {
			prefix = "👉"
		}
		tot, isSoft := hand.Score()
		softStr := ""
		if isSoft && tot < 21 {
			softStr = " (soft)"
		}
		sb.WriteString(fmt.Sprintf("%s Hand #%d: Bet: $%.2f | Total: %d%s | Status: %s\n",
			prefix, i+1, hand.Bet, tot, softStr, hand.Status.String()))
		sb.WriteString(r.RenderHandMultiLine(hand.Cards, false))
	}

	// 3. Oracle HUD
	if showOracle && counter != nil && advisor != nil {
		sb.WriteString("\n")
		sb.WriteString(FormatOracleHUD(counter, advisor, game.ActiveHand(), game.DealerUpcard()))
	}

	return sb.String()
}
