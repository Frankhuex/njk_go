package utext

import "testing"

func TestEscapeCQAnimatedSummary(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"image", "[CQ:image,file=a.jpg,summary=[动画表情]] 行", "[CQ:image,file=a.jpg,summary=&#91;动画表情&#93;] 行"},
		{"outside", "[动画表情] [普通文字]", "[动画表情] [普通文字]"},
		{"mixed", "[动画表情][CQ:image,summary=[动画表情]][CQ:face,id=12][动画表情]", "[动画表情][CQ:image,summary=&#91;动画表情&#93;][CQ:face,id=12][动画表情]"},
		{"escaped", "[CQ:image,summary=&#91;动画表情&#93;]", "[CQ:image,summary=&#91;动画表情&#93;]"},
		{"other label", "[CQ:image,summary=[其他]]", "[CQ:image,summary=[其他]]"},
		{"incomplete", "[CQ:image,summary=[动画表情]", "[CQ:image,summary=[动画表情]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := EscapeCQAnimatedSummary(tc.input); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			if got := EscapeCQAnimatedSummary(tc.want); got != tc.want {
				t.Fatalf("repeated escaping changed output: %q", got)
			}
		})
	}
}
