// Writes cal2pdf/names.go: the names of the months and the days of the week,
// and the first day of the week, of each language cal2pdf has, from CLDR, the
// Unicode Common Locale Data Repository, as Node's ICU carries it, not from
// us (the owner, 28 October 2026: languages by outside standards).
//
//   node tools/calendar-names.mjs > cal2pdf/names.go && gofmt -w cal2pdf/names.go
const languages = {
  "en-US": "English (US)", "en-GB": "English (UK)", "en-CA": "English (Canada)", "en-AU": "English (Australia)",
  "fr-FR": "Français", "fr-CA": "Français (Canada)", "de-DE": "Deutsch", "es-ES": "Español", "es-MX": "Español (México)",
  "it-IT": "Italiano", "pt-PT": "Português", "pt-BR": "Português (Brasil)", "nl-NL": "Nederlands",
  "sv-SE": "Svenska", "da-DK": "Dansk", "nb-NO": "Norsk bokmål", "fi-FI": "Suomi", "is-IS": "Íslenska",
  "pl-PL": "Polski", "cs-CZ": "Čeština", "sk-SK": "Slovenčina", "sl-SI": "Slovenščina", "hr-HR": "Hrvatski",
  "sr-RS": "Српски", "bg-BG": "Български", "ru-RU": "Русский", "uk-UA": "Українська", "el-GR": "Ελληνικά",
  "ro-RO": "Română", "hu-HU": "Magyar", "tr-TR": "Türkçe", "et-EE": "Eesti", "lv-LV": "Latviešu", "lt-LT": "Lietuvių",
  "ca-ES": "Català", "ga-IE": "Gaeilge",
};
const upperFirst = (s, tag) => s.charAt(0).toLocaleUpperCase(tag) + s.slice(1);
const out = [];
out.push("// names.go");
out.push("//");
out.push("// Copyright (c) 2026 PDFjet Software");
out.push("// Licensed under the MIT License. See LICENSE file in the project root.");
out.push("//");
out.push(`// Written by tools/calendar-names.mjs from CLDR ${process.versions.cldr}, as Node ${process.version}'s ICU`);
out.push("// carries it; not to be changed by hand.");
out.push("");
out.push("package main");
out.push("");
out.push("// The names of a language's calendar: the months as a calendar's heading");
out.push("// writes them, the days of the week short and in one letter or two, from");
out.push("// Sunday, and the day its weeks start on, 0 for Sunday and 1 for Monday.");
out.push("var languageNames = map[string]calendarNames{");
for (const [tag, label] of Object.entries(languages)) {
  const months = [], short = [], narrow = [];
  for (let m = 0; m < 12; m++) {
    // The month as a heading of a month and its year writes it, "January
    // 2027", its first letter a capital: the month alone is the form inside
    // a date in some languages, Greek's genitive among them
    const d = new Date(Date.UTC(2027, m, 15));
    const parts = new Intl.DateTimeFormat(tag, { month: "long", year: "numeric", timeZone: "UTC" }).formatToParts(d);
    months.push(upperFirst(parts.find((p) => p.type === "month").value, tag));
  }
  for (let w = 0; w < 7; w++) {
    // 2027-01-03 is a Sunday
    const d = new Date(Date.UTC(2027, 0, 3 + w));
    short.push(upperFirst(new Intl.DateTimeFormat(tag, { weekday: "short", timeZone: "UTC" }).format(d), tag));
    narrow.push(upperFirst(new Intl.DateTimeFormat(tag, { weekday: "narrow", timeZone: "UTC" }).format(d), tag));
  }
  const locale = new Intl.Locale(tag);
  const info = locale.getWeekInfo ? locale.getWeekInfo() : locale.weekInfo;
  const first = info.firstDay % 7; // 7 is Sunday
  const q = (a) => a.map((s) => JSON.stringify(s)).join(", ");
  out.push(`\t${JSON.stringify(tag)}: {label: ${JSON.stringify(label)}, firstDay: ${first},`);
  out.push(`\t\tmonths: [12]string{${q(months)}},`);
  out.push(`\t\tdays:   [7]string{${q(short)}},`);
  out.push(`\t\tletters: [7]string{${q(narrow)}}},`);
}
out.push("}");
console.log(out.join("\n"));
