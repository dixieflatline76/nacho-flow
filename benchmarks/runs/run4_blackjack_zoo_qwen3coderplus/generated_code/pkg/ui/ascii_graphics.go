package ui

import (
	"fmt"
	"strings"

	"blackjack/pkg/cards"
)

// PrintCard prints an ASCII representation of a single card
func PrintCard(card cards.Card) string {
	var top, middle, bottom string

	// Define the card borders
	top = "┌─────────┐"
	bottom = "└─────────┘"

	// Format the card content
	contentTop := fmt.Sprintf("%-2s", card.Rank)
	contentBottom := fmt.Sprintf("%2s", card.Rank)

	// Center the suit symbol
	suitLine := fmt.Sprintf(" %s ", card.Suit)

	middle = fmt.Sprintf("│%s       │\n│    %s    │\n│       %s│", contentTop, suitLine, contentBottom)

	return fmt.Sprintf("%s\n%s\n%s", top, middle, bottom)
}

// PrintHiddenCard prints an ASCII representation of a hidden/back card
func PrintHiddenCard() string {
	top := "┌─────────┐"
	middle := "│░░░░░░░░░│\n│░░░░░░░░░│\n│░░░░░░░░░│"
	bottom := "└─────────┘"

	return fmt.Sprintf("%s\n%s\n%s", top, middle, bottom)
}

// PrintHand prints multiple cards horizontally side by side
func PrintHand(hand []cards.Card, hideFirst bool) string {
	if len(hand) == 0 {
		return ""
	}

	// Create slices to hold each row of all cards
	var rows []string

	// Get the ASCII representation of each card
	cardStrings := make([][]string, len(hand))
	for i, card := range hand {
		var cardStr string
		if i == 0 && hideFirst {
			cardStr = PrintHiddenCard()
		} else {
			cardStr = PrintCard(card)
		}
		cardStrings[i] = strings.Split(cardStr, "\n")
	}

	// Combine the rows
	for rowIndex := 0; rowIndex < len(cardStrings[0]); rowIndex++ {
		var rowBuilder strings.Builder
		for cardIndex, cardRows := range cardStrings {
			rowBuilder.WriteString(cardRows[rowIndex])
			if cardIndex < len(cardStrings)-1 {
				rowBuilder.WriteString(" ") // Space between cards
			}
		}
		rows = append(rows, rowBuilder.String())
	}

	return strings.Join(rows, "\n")
}

// PrintCenteredText centers text within a specified width
func PrintCenteredText(text string, width int) string {
	textLen := len(text)
	if textLen >= width {
		return text
	}

	padding := (width - textLen) / 2
	return fmt.Sprintf("%s%s%s", strings.Repeat(" ", padding), text, strings.Repeat(" ", width-padding-textLen))
}

// PrintBox creates a bordered box with content
func PrintBox(content string, width int) string {
	lines := strings.Split(content, "\n")
	var result strings.Builder

	// Top border
	result.WriteString("┌" + strings.Repeat("─", width-2) + "┐\n")

	// Content with side borders
	for _, line := range lines {
		if len(line) > width-4 {
			line = line[:width-4] // Truncate if too long
		}
		result.WriteString(fmt.Sprintf("│ %s%s │\n", line, strings.Repeat(" ", width-4-len(line))))
	}

	// Bottom border
	result.WriteString("└" + strings.Repeat("─", width-2) + "┘")

	return result.String()
}

// PrintMenu prints a formatted menu
func PrintMenu(title string, options []string) string {
	var result strings.Builder

	result.WriteString(PrintCenteredText(title, 50) + "\n")
	result.WriteString(strings.Repeat("=", 50) + "\n")

	for i, option := range options {
		result.WriteString(fmt.Sprintf("%d. %s\n", i+1, option))
	}

	return result.String()
}

// PrintStatus displays the current game status
func PrintStatus(dealerHand []cards.Card, playerHand []cards.Card, playerName string, bet float64, isDealerTurn bool) string {
	var result strings.Builder

	result.WriteString("\n" + strings.Repeat("=", 60) + "\n")
	result.WriteString(PrintCenteredText("BLACKJACK TABLE", 60) + "\n")
	result.WriteString(strings.Repeat("=", 60) + "\n\n")

	// Dealer section
	result.WriteString("DEALER:\n")
	if isDealerTurn {
		result.WriteString(PrintHand(dealerHand, false)) // Show all cards during dealer's turn
	} else {
		result.WriteString(PrintHand(dealerHand, true)) // Hide first card initially
	}
	result.WriteString("\n\n")

	// Player section
	result.WriteString(fmt.Sprintf("%s (Bet: %.2f):\n", playerName, bet))
	result.WriteString(PrintHand(playerHand, false))
	result.WriteString("\n\n")

	return result.String()
}

// ClearScreen attempts to clear the terminal screen
func ClearScreen() {
	fmt.Print("\033[2J\033[H") // ANSI escape codes to clear screen and move cursor to top-left
}
