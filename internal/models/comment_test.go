package models

import (
	"math"
	"testing"
	"time"
)

// ----- Wilson Score Algorithm Tests -----

func TestWilsonScoreNoVotes(t *testing.T) {
	score := WilsonScore(0, 0)
	if score != 0 {
		t.Errorf("WilsonScore(0, 0) = %f, want 0", score)
	}
}

func TestWilsonScoreAllPositive(t *testing.T) {
	// 10 upvotes, 0 downvotes should produce a high score
	score := WilsonScore(10, 0)
	if score <= 0 || score > 1.0 {
		t.Errorf("WilsonScore(10, 0) = %f, want (0, 1]", score)
	}
	// Should be fairly confident positive
	if score < 0.65 {
		t.Errorf("WilsonScore(10, 0) = %f, expected >= 0.65 for 10 all-positive votes", score)
	}
}

func TestWilsonScoreAllNegative(t *testing.T) {
	// 0 upvotes, 10 downvotes should produce a very low score
	score := WilsonScore(0, 10)
	if score >= 0.35 {
		t.Errorf("WilsonScore(0, 10) = %f, expected < 0.35", score)
	}
}

func TestWilsonScoreEvenSplit(t *testing.T) {
	// 50/50 split should be around 0.5 but pulled down by confidence
	score := WilsonScore(50, 50)
	if score >= 0.5 {
		t.Errorf("WilsonScore(50, 50) = %f, expected < 0.5 (confidence pulls it below 0.5)", score)
	}
	if score < 0.3 {
		t.Errorf("WilsonScore(50, 50) = %f, expected > 0.3 (many votes should be somewhat confident)", score)
	}
}

func TestWilsonScoreConfidenceProperty(t *testing.T) {
	// KEY PROPERTY: More votes with the same ratio should produce a HIGHER score.
	// 1 up / 0 down (100% positive, n=1) should rank LOWER than
	// 100 up / 10 down (91% positive, n=110) because we're more confident
	// about the second comment's quality.
	fewVotes := WilsonScore(1, 0)
	manyVotes := WilsonScore(100, 10)

	if fewVotes >= manyVotes {
		t.Errorf(
			"WilsonScore confidence property violated: WilsonScore(1, 0)=%f >= WilsonScore(100, 10)=%f. "+
				"More votes with a good ratio should rank higher.",
			fewVotes, manyVotes,
		)
	}
}

func TestWilsonScoreSameRatioDifferentN(t *testing.T) {
	// Same 80/20 ratio. Larger sample → higher confidence → higher lower bound.
	small := WilsonScore(4, 1)   // n=5, 80% positive
	large := WilsonScore(80, 20) // n=100, 80% positive

	if small >= large {
		t.Errorf(
			"WilsonScore(4,1)=%f >= WilsonScore(80,20)=%f. "+
				"Larger sample with same ratio should have higher lower bound.",
			small, large,
		)
	}
}

func TestWilsonScoreMonotonicity(t *testing.T) {
	// Adding an upvote should increase the score
	before := WilsonScore(10, 5)
	after := WilsonScore(11, 5)
	if after <= before {
		t.Errorf("Adding an upvote should increase score: before=%f, after=%f", before, after)
	}

	// Adding a downvote should decrease the score
	before2 := WilsonScore(10, 5)
	after2 := WilsonScore(10, 6)
	if after2 >= before2 {
		t.Errorf("Adding a downvote should decrease score: before=%f, after=%f", before2, after2)
	}
}

func TestWilsonScoreBoundedZeroToOne(t *testing.T) {
	// Wilson score should always be in [0, 1] for non-negative inputs
	testCases := [][2]int64{
		{0, 0}, {1, 0}, {0, 1}, {1, 1},
		{100, 0}, {0, 100}, {100, 100},
		{1000, 1}, {1, 1000},
		{999999, 1}, {1, 999999},
	}
	for _, tc := range testCases {
		score := WilsonScore(tc[0], tc[1])
		if score < 0 || score > 1.0 {
			t.Errorf("WilsonScore(%d, %d) = %f, out of [0, 1] range", tc[0], tc[1], score)
		}
	}
}

func TestWilsonScoreKnownValue(t *testing.T) {
	// Verify against a known manual calculation.
	// For up=600, down=400: p = 0.6, n = 1000, z = 1.96
	// denominator = 1 + 1.96^2/1000 = 1.003842
	// centre = 0.6 + 1.96^2/2000 = 0.601921
	// spread = 1.96 * sqrt((0.6*0.4 + 1.96^2/4000000)/1000) = 1.96 * sqrt(0.000240001) ≈ 0.030365
	// lower = (0.601921 - 0.030365) / 1.003842 ≈ 0.56936
	score := WilsonScore(600, 400)
	expected := 0.56936
	if math.Abs(score-expected) > 0.001 {
		t.Errorf("WilsonScore(600, 400) = %f, expected ~%f (tolerance 0.001)", score, expected)
	}
}

// ----- Controversy Score Tests -----

func TestControversyScoreNoVotes(t *testing.T) {
	score := ControversyScore(0, 0)
	if score != 0 {
		t.Errorf("ControversyScore(0, 0) = %f, want 0", score)
	}
}

func TestControversyScoreHighControversy(t *testing.T) {
	// 50 up, 50 down → total=100, |net|=0 → capped to 1 → score = 100
	score := ControversyScore(50, 50)
	if score != 100.0 {
		t.Errorf("ControversyScore(50, 50) = %f, want 100", score)
	}
}

func TestControversyScoreLowControversy(t *testing.T) {
	// 100 up, 0 down → total=100, |net|=100 → score = 1
	score := ControversyScore(100, 0)
	if score != 1.0 {
		t.Errorf("ControversyScore(100, 0) = %f, want 1", score)
	}
}

func TestControversyScoreOrdering(t *testing.T) {
	// More even split with more votes should be more controversial
	divisive := ControversyScore(500, 490)   // very controversial
	unanimous := ControversyScore(500, 0)    // not controversial
	lowVolume := ControversyScore(5, 5)      // controversial but few votes

	if divisive <= unanimous {
		t.Errorf("divisive(%f) should rank above unanimous(%f)", divisive, unanimous)
	}
	if divisive <= lowVolume {
		t.Errorf("high-volume divisive(%f) should rank above low-volume divisive(%f)", divisive, lowVolume)
	}
}

// ----- Sort Order Validation Tests -----

func TestValidSortOrderAccepted(t *testing.T) {
	valid := []CommentSortOrder{
		SortBest, SortNew, SortTop,
		SortControversial, SortOld,
	}
	for _, s := range valid {
		if !ValidSortOrder(s) {
			t.Errorf("ValidSortOrder(%q) = false, want true", s)
		}
	}
}

func TestValidSortOrderRejected(t *testing.T) {
	invalid := []CommentSortOrder{"", "random", "hot", "rising", "BEST"}
	for _, s := range invalid {
		if ValidSortOrder(s) {
			t.Errorf("ValidSortOrder(%q) = true, want false", s)
		}
	}
}

// ----- Comment Model Field Defaults -----

func TestCommentModelDefaults(t *testing.T) {
	c := Comment{}
	if c.Upvotes != 0 {
		t.Errorf("default Upvotes = %d, want 0", c.Upvotes)
	}
	if c.Downvotes != 0 {
		t.Errorf("default Downvotes = %d, want 0", c.Downvotes)
	}
	if c.Score != 0 {
		t.Errorf("default Score = %f, want 0", c.Score)
	}
	if c.Depth != 0 {
		t.Errorf("default Depth = %d, want 0", c.Depth)
	}
	if c.IsDeleted {
		t.Error("default IsDeleted = true, want false")
	}
	if c.EditedAt != nil {
		t.Error("default EditedAt should be nil")
	}
	if c.ParentID != nil {
		t.Error("default ParentID should be nil (top-level)")
	}
}

func TestCommentVoteValues(t *testing.T) {
	// Ensure int8 can hold +1 and -1
	upvote := CommentVote{Value: 1}
	downvote := CommentVote{Value: -1}

	if upvote.Value != 1 {
		t.Errorf("upvote.Value = %d, want 1", upvote.Value)
	}
	if downvote.Value != -1 {
		t.Errorf("downvote.Value = %d, want -1", downvote.Value)
	}
}

// ----- Constants Tests -----

func TestMaxCommentBodyLength(t *testing.T) {
	if MaxCommentBodyLength <= 0 {
		t.Errorf("MaxCommentBodyLength = %d, must be positive", MaxCommentBodyLength)
	}
	if MaxCommentBodyLength > 100000 {
		t.Errorf("MaxCommentBodyLength = %d, unreasonably large", MaxCommentBodyLength)
	}
}

func TestEditGracePeriod(t *testing.T) {
	if EditGracePeriod <= 0 {
		t.Error("EditGracePeriod must be positive")
	}
	if EditGracePeriod > 10*time.Minute {
		t.Error("EditGracePeriod seems unreasonably long")
	}
}

func TestMaxTreeDepth(t *testing.T) {
	if MaxTreeDepth < 3 {
		t.Errorf("MaxTreeDepth = %d, too shallow for useful threading", MaxTreeDepth)
	}
	if MaxTreeDepth > 100 {
		t.Errorf("MaxTreeDepth = %d, unreasonably deep", MaxTreeDepth)
	}
}

// ----- Comment Depth Calculation -----

func TestCommentReplyDepth(t *testing.T) {
	// Simulate depth chain: top → reply → nested reply
	parentID := uint(1)
	parent := Comment{ID: 1, Depth: 0}
	child := Comment{ID: 2, ParentID: &parentID, Depth: parent.Depth + 1}
	childID := uint(2)
	grandchild := Comment{ID: 3, ParentID: &childID, Depth: child.Depth + 1}

	if parent.Depth != 0 {
		t.Errorf("top-level depth = %d, want 0", parent.Depth)
	}
	if child.Depth != 1 {
		t.Errorf("reply depth = %d, want 1", child.Depth)
	}
	if grandchild.Depth != 2 {
		t.Errorf("nested reply depth = %d, want 2", grandchild.Depth)
	}
}

// ----- Wilson Score Edge Cases -----

func TestWilsonScoreSingleUpvote(t *testing.T) {
	// A single upvote should give a positive but conservative score
	score := WilsonScore(1, 0)
	if score <= 0 {
		t.Errorf("WilsonScore(1, 0) = %f, should be positive", score)
	}
	// Should be much less than 1.0 due to low confidence
	if score > 0.8 {
		t.Errorf("WilsonScore(1, 0) = %f, should be < 0.8 (low confidence)", score)
	}
}

func TestWilsonScoreSingleDownvote(t *testing.T) {
	score := WilsonScore(0, 1)
	// Should be very low — zero positive out of 1 total
	if score > 0.2 {
		t.Errorf("WilsonScore(0, 1) = %f, should be < 0.2", score)
	}
	if score < 0 {
		t.Errorf("WilsonScore(0, 1) = %f, should be >= 0", score)
	}
}

func TestWilsonScoreLargeNPositive(t *testing.T) {
	// With many positive votes, score should approach 1.0
	score := WilsonScore(10000, 0)
	if score < 0.99 {
		t.Errorf("WilsonScore(10000, 0) = %f, expected > 0.99 for many positive votes", score)
	}
}

func TestWilsonScoreSymmetry(t *testing.T) {
	// WilsonScore is NOT symmetric: WilsonScore(a, b) != 1 - WilsonScore(b, a)
	// But for equal votes, it should be below 0.5
	score := WilsonScore(10, 10)
	if score >= 0.5 {
		t.Errorf("WilsonScore(10, 10) = %f, expected < 0.5", score)
	}
}
