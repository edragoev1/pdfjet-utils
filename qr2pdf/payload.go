// payload.go
//
// Copyright (c) 2026 PDFjet Software
// Licensed under the MIT License. See LICENSE file in the project root.
//
// What a QR code holds, written as phones read it: a link, a text, a Wi-Fi
// network (the WIFI: of the ZXing project, which the cameras of Android and
// iPhone join), a contact card (vCard 3.0, RFC 2426), an email (mailto:,
// RFC 6068) or a phone number (tel:, RFC 3966).

package main

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// A link that has no scheme, a name with a dot, as pdfjet.com or
// www.example.com/menu, which people type as they read it
var bareHost = regexp.MustCompile(`^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+([/?#].*)?$`)

// linkOf returns the link as the code holds it: https:// before a bare name.
func linkOf(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("there is no link")
	}
	if bareHost.MatchString(text) {
		text = "https://" + text
	}
	u, err := url.Parse(text)
	if err != nil || u.Scheme == "" {
		return "", fmt.Errorf("%q is not a link, as https://example.com", text)
	}
	return text, nil
}

// wifiEscape escapes what the WIFI: format gives a meaning to.
func wifiEscape(text string) string {
	return strings.NewReplacer(`\`, `\\`, `;`, `\;`, `,`, `\,`, `:`, `\:`, `"`, `\"`).Replace(text)
}

// wifiOf returns the code of a Wi-Fi network: its name, its password and its
// security, WPA (WPA2 and WPA3 too), WEP or none, and whether it is hidden.
func wifiOf(name, password, security string, hidden bool) (string, error) {
	if name == "" {
		return "", fmt.Errorf("the network has no name")
	}
	security = strings.ToUpper(security)
	switch security {
	case "", "WPA":
		security = "WPA"
	case "WEP":
	case "NONE", "NOPASS":
		security, password = "nopass", ""
	default:
		return "", fmt.Errorf("the security %q is not WPA, WEP or none", security)
	}
	if security != "nopass" && password == "" {
		return "", fmt.Errorf("the network has no password: choose None if it has none")
	}
	var b strings.Builder
	b.WriteString("WIFI:T:" + security + ";S:" + wifiEscape(name) + ";")
	if password != "" {
		b.WriteString("P:" + wifiEscape(password) + ";")
	}
	if hidden {
		b.WriteString("H:true;")
	}
	b.WriteString(";")
	return b.String(), nil
}

// vcardEscape escapes a value of vCard 3.0.
func vcardEscape(text string) string {
	return strings.NewReplacer(`\`, `\\`, `;`, `\;`, `,`, `\,`, "\r\n", `\n`, "\n", `\n`).Replace(text)
}

// contact is a contact card's fields.
type contact struct {
	first, last, organization, title, phone, email, website string
}

// vcardOf returns the contact's card, vCard 3.0, its lines ending in CR LF.
func vcardOf(c contact) (string, error) {
	if strings.TrimSpace(c.first+c.last+c.organization) == "" {
		return "", fmt.Errorf("the contact has no name")
	}
	lines := []string{"BEGIN:VCARD", "VERSION:3.0",
		"N:" + vcardEscape(c.last) + ";" + vcardEscape(c.first) + ";;;"}
	full := strings.TrimSpace(c.first + " " + c.last)
	if full == "" {
		full = c.organization
	}
	lines = append(lines, "FN:"+vcardEscape(full))
	for _, f := range []struct{ key, value string }{
		{"ORG", c.organization}, {"TITLE", c.title}, {"TEL;TYPE=CELL", c.phone},
		{"EMAIL", c.email}, {"URL", c.website},
	} {
		if v := strings.TrimSpace(f.value); v != "" {
			lines = append(lines, f.key+":"+vcardEscape(v))
		}
	}
	lines = append(lines, "END:VCARD")
	return strings.Join(lines, "\r\n"), nil
}

// emailOf returns mailto: of the address, with its subject if any.
func emailOf(address, subject string) (string, error) {
	address = strings.TrimSpace(address)
	if !strings.Contains(address, "@") || strings.ContainsAny(address, " <>") {
		return "", fmt.Errorf("%q is not an email address", address)
	}
	text := "mailto:" + address
	if subject = strings.TrimSpace(subject); subject != "" {
		text += "?subject=" + strings.ReplaceAll(url.QueryEscape(subject), "+", "%20")
	}
	return text, nil
}

// phoneOf returns tel: of the number, its digits and its +.
func phoneOf(number string) (string, error) {
	var b strings.Builder
	for i, r := range strings.TrimSpace(number) {
		if r >= '0' && r <= '9' || r == '+' && i == 0 {
			b.WriteRune(r)
		}
	}
	digits := strings.TrimPrefix(b.String(), "+")
	if len(digits) < 3 || len(digits) > 15 {
		return "", fmt.Errorf("%q is not a phone number", number)
	}
	return "tel:" + b.String(), nil
}
