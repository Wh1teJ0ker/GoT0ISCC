package theory

import (
	"net/url"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func mustParseTheoryHTML(t *testing.T, raw string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parse html: %v", err)
	}
	return doc
}

func TestValidateTheoryPaperSnapshotRejectsLoginPage(t *testing.T) {
	doc := mustParseTheoryHTML(t, `
<html>
  <body>
    <form action="/login" method="post">
      <input name="name" />
      <input type="password" name="password" />
      <button>登录</button>
      <a>忘记密码</a>
    </form>
  </body>
</html>`)

	question := extractTheoryQuestion(doc)
	if question.Title == "" {
		t.Fatalf("expected legacy parser to extract a misleading title")
	}
	if err := validateTheoryPaperSnapshot(doc, question, "nonce", "1"); err != errTheorySessionInvalid {
		t.Fatalf("expected session invalid, got %v", err)
	}
}

func TestValidateTheoryPaperSnapshotRequiresSubmitFields(t *testing.T) {
	doc := mustParseTheoryHTML(t, `
<html>
  <body>
    <strong>第1题</strong>
    <div>题目文本</div>
    <form action="/paper">
      <input type="radio" name="option" value="A" /> A. 选项A
      <input type="radio" name="option" value="B" /> B. 选项B
    </form>
  </body>
</html>`)

	question := extractTheoryQuestion(doc)
	if !isValidTheoryQuestion(question) {
		t.Fatalf("expected parsed question to be valid: %#v", question)
	}
	if err := validateTheoryPaperSnapshot(doc, question, "", "1"); err == nil {
		t.Fatalf("expected missing nonce error")
	}
}

func TestTheoryProgressPageExtractsScoreWithoutQuestion(t *testing.T) {
	doc := mustParseTheoryHTML(t, `
<html>
  <body>
    <div>理论题已完成</div>
    <div>进度 100/100</div>
    <div>当前得分：98 / 100</div>
  </body>
</html>`)

	question := extractTheoryQuestion(doc)
	if isValidTheoryQuestion(question) {
		t.Fatalf("expected completion page to be non-answerable")
	}
	if got := extractTheoryQuestionNumber(doc); got != 100 {
		t.Fatalf("expected progress number 100, got %d", got)
	}
	current, total, text := extractTheoryScore(doc)
	if current != "98" || total != "100" || text != "98 / 100" {
		t.Fatalf("unexpected score current=%q total=%q text=%q", current, total, text)
	}
	if msg := extractTheoryProgressMessage(doc); !strings.Contains(msg, "已完成") {
		t.Fatalf("expected completion message, got %q", msg)
	}
}

func TestTheoryRealCompletionPageExtractsScore(t *testing.T) {
	doc := mustParseTheoryHTML(t, `
<html>
  <body>
    <div>恭喜您完成选择题答题，您的得分是460。</div>
    <a>点击这里</a>
    <span>开始闯关。</span>
  </body>
</html>`)

	question := extractTheoryQuestion(doc)
	if isValidTheoryQuestion(question) {
		t.Fatalf("expected real completion page to be non-answerable")
	}
	current, total, text := extractTheoryScore(doc)
	if current != "460" || total != "" || text != "460" {
		t.Fatalf("unexpected score current=%q total=%q text=%q", current, total, text)
	}
	if msg := extractTheoryProgressMessage(doc); !strings.Contains(msg, "已完成") {
		t.Fatalf("expected completion message, got %q", msg)
	}
}

func TestExtractTheoryAnswerFormUsesRealFieldNames(t *testing.T) {
	doc := mustParseTheoryHTML(t, `
<html>
  <body>
    <div>第12题 关于网络协议，下列说法正确的是？</div>
    <form action="/paper/submit" method="post">
      <input type="hidden" name="csrf_nonce" value="nonce-123" />
      <input type="hidden" name="question_number" value="12" />
      <label for="opt-a"><input id="opt-a" type="checkbox" name="answer" value="A" /> A. 选项A</label>
      <label for="opt-b"><input id="opt-b" type="checkbox" name="answer" value="B" /> B. 选项B</label>
    </form>
  </body>
</html>`)

	form := extractTheoryAnswerForm(doc)
	if form.Action != "/paper/submit" {
		t.Fatalf("unexpected action %q", form.Action)
	}
	if form.Method != "POST" {
		t.Fatalf("unexpected method %q", form.Method)
	}
	if form.NonceField != "csrf_nonce" || form.Nonce != "nonce-123" {
		t.Fatalf("unexpected nonce extraction %#v", form)
	}
	if form.NumberField != "question_number" || form.NumberValue != "12" {
		t.Fatalf("unexpected number extraction %#v", form)
	}
	if form.OptionField != "answer" {
		t.Fatalf("unexpected option field %#v", form)
	}
	if !form.AllowsMultiple {
		t.Fatalf("expected checkbox form to allow multiple")
	}
}

func TestExtractTheoryQuestionFindsLabelWrappedOptions(t *testing.T) {
	doc := mustParseTheoryHTML(t, `
<html>
  <body>
    <div>第8题 下面哪些属于安全编码实践？</div>
    <form action="/paper">
      <input type="hidden" name="nonce" value="abc" />
      <input type="hidden" name="number" value="8" />
      <label><input type="checkbox" name="option" value="A" /> A. 参数化查询</label>
      <label><input type="checkbox" name="option" value="B" /> B. 拼接 SQL</label>
    </form>
  </body>
</html>`)

	question := extractTheoryQuestion(doc)
	if question.Title == "" {
		t.Fatalf("expected question title")
	}
	if question.SelectionType != "multiple" {
		t.Fatalf("unexpected selection type %#v", question)
	}
	if len(question.Options) != 2 {
		t.Fatalf("unexpected options %#v", question.Options)
	}
	if question.Options[0].Content != "参数化查询" || question.Options[1].Content != "拼接 SQL" {
		t.Fatalf("unexpected option labels %#v", question.Options)
	}
}

func TestEncodeTheoryLoginFormEscapesPlusInPassword(t *testing.T) {
	raw := encodeTheoryLoginForm("BitFlux", "abc+123")
	if strings.Contains(raw, "abc+123") {
		t.Fatalf("password plus was not escaped: %s", raw)
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	if got := values.Get("password"); got != "abc+123" {
		t.Fatalf("expected password to round-trip with plus, got %q", got)
	}
}
