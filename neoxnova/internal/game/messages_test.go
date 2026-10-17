package game

import (
	"strings"
	"testing"
)

func TestFormatAmount(t *testing.T) {
	cases := map[int64]string{
		0:             "0",
		7:             "7",
		999:           "999",
		1000:          "1.000",
		1009380993501: "1.009.380.993.501",
		-1234567:      "-1.234.567",
	}
	for in, want := range cases {
		if got := FormatAmount(in); got != want {
			t.Errorf("FormatAmount(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestWelcomeMessage(t *testing.T) {
	sender, subject, body := WelcomeMessage()
	if sender != MessageSenderAdministration {
		t.Errorf("welcome sender = %q, want %q", sender, MessageSenderAdministration)
	}
	if subject == "" || body == "" {
		t.Fatal("welcome subject/body must be non-empty")
	}
}

func TestTransportMessages(t *testing.T) {
	sender, subject, body := TransportDeliveryMessage(3, 125, 12, 1009380993501, 112906820973, 74799832)
	if sender != MessageSenderFleetHQ || subject != "Located fleet" {
		t.Errorf("delivery sender/subject = %q/%q", sender, subject)
	}
	for _, want := range []string{"[3:125:12]", "1.009.380.993.501 Metal", "112.906.820.973 Crystal", "74.799.832 Deuterium"} {
		if !strings.Contains(body, want) {
			t.Errorf("delivery body %q missing %q", body, want)
		}
	}

	_, subject, body = TransportReturnMessage("Gotijy", 3, 125, 12)
	if subject != "Fleet return" || !strings.Contains(body, "Gotijy [3:125:12]") {
		t.Errorf("return message = %q / %q", subject, body)
	}
}

func TestMessageSenderForCategory(t *testing.T) {
	cases := map[int]string{
		MessageCategoryPlayer:       MessageSenderAdministration,
		MessageCategoryAlliance:     MessageSenderUniverse,
		MessageCategorySystem:       MessageSenderSystem,
		MessageCategoryConstruction: MessageSenderSystem,
		MessageCategoryGame:         MessageSenderSystem,
		MessageCategorySpy:          MessageSenderRoomMonitoring,
		MessageCategoryTransport:    MessageSenderFleetHQ,
	}
	for cat, want := range cases {
		if got := MessageSenderForCategory(cat); got != want {
			t.Errorf("MessageSenderForCategory(%d) = %q, want %q", cat, got, want)
		}
	}
}
