package savestate

import "testing"

func TestVerifyMinimalSavestate(t *testing.T) {
	bytes, err := BuildMinimalBizHawkSavestate()
	if err != nil {
		t.Fatal(err)
	}
	if !IsProbablyBizHawkSavestate(bytes) {
		t.Fatal("expected probable savestate")
	}
	result := VerifyBizHawkSavestate(bytes, VerifyOptions{})
	if !result.OK {
		t.Fatalf("%+v", result)
	}
	if result.FormatVersion != "1.0.3" || result.ZipSubVersion != 3 {
		t.Fatalf("version %+v", result)
	}
}

func TestRejectInvalidZip(t *testing.T) {
	if IsProbablyBizHawkSavestate(InvalidSaveZip) {
		t.Fatal("expected false")
	}
	result := VerifyBizHawkSavestate(InvalidSaveZip, VerifyOptions{})
	if result.OK || result.Code != CodeNotZipSavestate {
		t.Fatalf("%+v", result)
	}
}

func TestRejectNonZip(t *testing.T) {
	result := VerifyBizHawkSavestate([]byte{1, 2, 3, 4}, VerifyOptions{})
	if result.OK || result.Code != CodeNotZipSavestate {
		t.Fatalf("%+v", result)
	}
}
