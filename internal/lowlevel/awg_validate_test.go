package lowlevel

import "testing"

func TestAWGParams_Validate(t *testing.T) {
	t.Run("valid params", func(t *testing.T) {
		p := AWGParams{
			Jc:4, Jmin:70, Jmax:600,
			S1:25, S2:35, S3:10, S4:5,
			H1:111, H2:222, H3:333, H4:444,
		}
		if err := p.Validate(); err != nil {
			t.Fatalf("expected valid params, got error: %v", err)
		}
	})

	t.Run("Jc zero allowed", func(t *testing.T) {
		p := AWGParams{
			Jc:0, Jmin:70, Jmax:600,
			S1:25, S2:35, S3:10, S4:5,
			H1:111, H2:222, H3:333, H4:444,
		}
		if err := p.Validate(); err != nil {
			t.Fatalf("expected Jc=0 to be valid, got: %v", err)
		}
	})

	t.Run("Jc too high", func(t *testing.T) {
		p := AWGParams{
			Jc:999, Jmin:70, Jmax:600,
			S1:25, S2:35, S3:10, S4:5,
			H1:111, H2:222, H3:333, H4:444,
		}
		if err := p.Validate(); err == nil {
			t.Fatal("expected error for Jc=999, got nil")
		}
	})

	t.Run("Jmax not greater than Jmin when Jc>0", func(t *testing.T) {
		p := AWGParams{
			Jc:1, Jmin:500, Jmax:100,
			S1:25, S2:35, S3:10, S4:5,
			H1:111, H2:222, H3:333, H4:444,
		}
		if err := p.Validate(); err == nil {
			t.Fatal("expected error for Jmax<Jmin when Jc>0, got nil")
		}
	})

	t.Run("S1 out of range", func(t *testing.T) {
		p := AWGParams{
			Jc:4, Jmin:70, Jmax:600,
			S1:9999, S2:35, S3:10, S4:5,
			H1:111, H2:222, H3:333, H4:444,
		}
		if err := p.Validate(); err == nil {
			t.Fatal("expected error for S1=9999, got nil")
		}
	})

	t.Run("duplicate H", func(t *testing.T) {
		p := AWGParams{
			Jc:4, Jmin:70, Jmax:600,
			S1:25, S2:35, S3:10, S4:5,
			H1:100, H2:100, H3:300, H4:400,
		}
		if err := p.Validate(); err == nil {
			t.Fatal("expected error for duplicate H, got nil")
		}
	})

	t.Run("H below 5", func(t *testing.T) {
		p := AWGParams{
			Jc:4, Jmin:70, Jmax:600,
			S1:25, S2:35, S3:10, S4:5,
			H1:1, H2:200, H3:300, H4:400,
		}
		if err := p.Validate(); err == nil {
			t.Fatal("expected error for H<5, got nil")
		}
	})
}
