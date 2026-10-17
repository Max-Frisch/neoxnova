package game

import (
	"fmt"
	"strconv"
	"strings"
)

// Message categories mirror the live client's message `cat` ids. The report
// categories (spy / combat / expedition) are backed by their own structured
// tables; the rest are free-form rows in the messages table.
const (
	MessageCategorySpy          = 0
	MessageCategoryPlayer       = 1
	MessageCategoryAlliance     = 2
	MessageCategoryCombat       = 3
	MessageCategorySystem       = 4
	MessageCategoryTransport    = 5
	MessageCategoryExpedition   = 15
	MessageCategoryGame         = 50
	MessageCategoryConstruction = 99
)

// Message senders, named as in the live client for each category.
const (
	MessageSenderAdministration = "Administration"
	MessageSenderSystem         = "System"
	MessageSenderFleetHQ        = "Fleet Headquarters"
	MessageSenderUniverse       = "Universe"
	MessageSenderRoomMonitoring = "Room monitoring"
)

// MessageSenderForCategory returns the display sender of a category.
func MessageSenderForCategory(category int) string {
	switch category {
	case MessageCategoryPlayer:
		return MessageSenderAdministration
	case MessageCategoryAlliance:
		return MessageSenderUniverse
	case MessageCategorySystem, MessageCategoryGame, MessageCategoryConstruction:
		return MessageSenderSystem
	case MessageCategorySpy:
		return MessageSenderRoomMonitoring
	default: // combat, expedition, transport
		return MessageSenderFleetHQ
	}
}

// WelcomeMessage is the onboarding private message every new account receives.
func WelcomeMessage() (sender, subject, body string) {
	return MessageSenderAdministration, "Welcome",
		"Welcome to Neo-XNova! Your homeworld is ready. Start by building a Metal " +
			"Mine, then a Solar Plant to power it, and raise your Research Lab to " +
			"unlock the tech tree. Good luck, Commander!"
}

// TransportDeliveryMessage is the "Located fleet" notice sent when a transport
// reaches its destination and unloads its cargo.
func TransportDeliveryMessage(galaxy, system, position int, metal, crystal, deuterium int64) (sender, subject, body string) {
	return MessageSenderFleetHQ, "Located fleet", fmt.Sprintf(
		"Your fleet reaches the planet [%d:%d:%d] and delivers %s Metal, %s Crystal and %s Deuterium.",
		galaxy, system, position, FormatAmount(metal), FormatAmount(crystal), FormatAmount(deuterium))
}

// TransportReturnMessage is the "Fleet return" notice sent when a transport
// bound for another planet turns around and heads home.
func TransportReturnMessage(name string, galaxy, system, position int) (sender, subject, body string) {
	return MessageSenderFleetHQ, "Fleet return", fmt.Sprintf(
		"One of your fleets returns from the planet %s [%d:%d:%d].", name, galaxy, system, position)
}

// FormatAmount renders an integer with '.' thousands separators, matching the
// live client's in-message number style (e.g. 1.009.380).
func FormatAmount(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}
