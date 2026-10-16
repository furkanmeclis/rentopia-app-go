package ubl

import "testing"

func TestFutureOnlyInUTC(t *testing.T) {
	msg := "Geçersiz cbc:IssueDate değeri : '2026-10-17' cbc:IssueDate alanı günün tarihinden ileri bir tarih olamaz"
	if !futureOnlyInUTC(msg, "2026-10-17") {
		t.Fatal("an issue date equal to the Istanbul day must pass")
	}
	if futureOnlyInUTC(msg, "2026-10-16") {
		t.Fatal("an issue date after the Istanbul day must still fail")
	}
	other := "Geçersiz cbc:IssueDate değeri : '2004-01-01' cbc:IssueDate alanı 01.01.2005 tarihinden önce bir tarih olamaz"
	if futureOnlyInUTC(other, "2026-10-17") {
		t.Fatal("other assertions must not be swallowed")
	}
}
