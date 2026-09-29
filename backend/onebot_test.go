package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func segment(typ string, data map[string]any) map[string]any {
	return map[string]any{"type": typ, "data": data}
}

func TestOneBotFrameAndMessageParsing(t *testing.T) {
	o := NewOneBot()
	var events []GroupMessage
	o.onEvent = func(gm GroupMessage) { events = append(events, gm) }
	o.handleFrame([]byte(`{"post_type":"meta_event","self_id":99}`))
	o.handleFrame([]byte(`{"post_type":"message","message_type":"private","self_id":99}`))
	o.handleFrame([]byte(`{"post_type":"message","message_type":"group","self_id":99,"group_id":42,"user_id":7,"message_id":8,"sender":{"card":"班长","nickname":"原昵称","role":"admin"},"message":[{"type":"text","data":{"text":"请看 "}},{"type":"at","data":{"qq":"all"}},{"type":"at","data":{"qq":"99"}},{"type":"image","data":{}}]}`))
	if o.SelfID() != 99 || len(events) != 1 {
		t.Fatalf("self=%d events=%v", o.SelfID(), events)
	}
	gm := events[0]
	if gm.GroupID != 42 || gm.UserID != 7 || gm.Nickname != "班长" || gm.Role != "admin" || !gm.AtAll || !gm.AtSelf || !gm.HasImage || !strings.Contains(gm.Text, "@99") {
		t.Fatalf("parsed group message: %+v", gm)
	}
	if got := parseGroupMessage(map[string]any{"message": "hello"}, 0); got.Role != "member" || got.Text != "hello" {
		t.Fatalf("string fallback: %+v", got)
	}
}

func TestOneBotSuppressesDuplicateMessageIDs(t *testing.T) {
	o := NewOneBot()
	var events int
	o.onEvent = func(GroupMessage) { events++ }
	frame := `{"post_type":"message","message_type":"group","group_id":42,"user_id":7,"message_id":8,"message":"通知"}`
	o.handleFrame([]byte(frame))
	o.handleFrame([]byte(frame))
	if events != 1 {
		t.Fatalf("duplicate message emitted %d events", events)
	}
}

func TestOneBotSegmentsAndFiles(t *testing.T) {
	msg := []any{
		segment("text", map[string]any{"text": "通知"}),
		segment("at", map[string]any{"qq": "all"}),
		segment("at", map[string]any{"qq": "123"}),
		segment("image", map[string]any{"url": "https://x.qpic.cn/p", "sub_type": "1"}),
		segment("record", map[string]any{"file": "voice.amr"}),
		segment("reply", map[string]any{"id": "88"}),
		segment("file", map[string]any{"file": "讲义.pdf", "file_unique": "file-1", "file_size": 1234.0, "busid": 4.0}),
	}
	segs := buildSegments(msg, 0, func(id int64) string {
		if id == 123 {
			return "小王"
		}
		return ""
	},
		func(id int64) *ReplyQuote {
			if id != 88 {
				t.Errorf("quote id=%d", id)
			}
			return &ReplyQuote{Text: "原通知"}
		})
	if len(segs) != 6 || segs[2].Name != "小王" || segs[3].Type != "image" || !segs[3].Sticker || segs[4].File != "voice.amr" || segs[5].Reply.Text != "原通知" {
		t.Fatalf("segments = %+v", segs)
	}
	files := extractFiles(msg)
	if len(files) != 1 || files[0].FileID != "file-1" || files[0].Name != "讲义.pdf" || files[0].Size != 1234 {
		t.Fatalf("files = %+v", files)
	}
	if got := buildSegments("一句话", 0, nil, nil); !reflect.DeepEqual(got, []MsgSegment{{Type: "text", Text: "一句话"}}) {
		t.Fatalf("string segments = %+v", got)
	}
}

func TestOneBotHTTPContractAndHistory(t *testing.T) {
	var historyParams map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("bad OneBot request: %s auth=%q", r.Method, r.Header.Get("Authorization"))
		}
		var params map[string]any
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			t.Error(err)
		}
		switch r.URL.Path {
		case "/get_group_msg_history":
			historyParams = params
			fmt.Fprint(w, `{"status":"ok","retcode":0,"data":{"messages":[{"message_id":9,"real_seq":77,"user_id":99,"time":100,"sender":{"card":"群名片","role":"owner"},"message":[{"type":"at","data":{"qq":"123"}},{"type":"reply","data":{"id":"88"}},{"type":"file","data":{"file":"表格.xlsx","file_id":"file-2"}}]}]}}`)
		case "/get_group_member_list":
			fmt.Fprint(w, `{"status":"ok","retcode":0,"data":[{"user_id":123,"card":"小王","nickname":"王"}]}`)
		case "/get_msg":
			fmt.Fprint(w, `{"status":"ok","retcode":0,"data":{"group_id":42,"user_id":5,"sender":{"nickname":"老师"},"message":"明日集合"}}`)
		case "/get_record":
			fmt.Fprintf(w, `{"status":"ok","retcode":0,"data":{"base64":%q}}`, "data:audio/mp3;base64,"+base64.StdEncoding.EncodeToString([]byte("MP3")))
		case "/get_group_file_url":
			fmt.Fprint(w, `{"status":"ok","retcode":0,"data":{"url":"https://example.test/file"}}`)
		default:
			t.Errorf("unexpected action %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	o := NewOneBot()
	o.httpBase, o.token = srv.URL, "token"
	o.selfID.Store(99)
	msgs, err := o.GetGroupMsgHistory(42, 99, 77)
	if err != nil {
		t.Fatal(err)
	}
	if historyParams["count"] != float64(30) || historyParams["message_seq"] != float64(77) || historyParams["reverseOrder"] != true {
		t.Fatalf("paging contract: %#v", historyParams)
	}
	if len(msgs) != 1 || msgs[0].MessageSeq != 77 || !msgs[0].IsSelf || msgs[0].Segments[0].Name != "小王" || msgs[0].Segments[1].Reply.Text != "明日集合" || msgs[0].Files[0].FileID != "file-2" {
		t.Fatalf("history = %+v", msgs)
	}
	if u, err := o.GetGroupFileURL(42, "file-2", 0); err != nil || u != "https://example.test/file" {
		t.Fatalf("file URL %q err=%v", u, err)
	}
	if audio, err := o.GetRecordMP3("voice.amr"); err != nil || string(audio) != "MP3" {
		t.Fatalf("audio=%q err=%v", audio, err)
	}
}

func TestOneBotCallRejectsFailureEnvelope(t *testing.T) {
	for _, body := range []string{
		`{"status":"failed","retcode":0,"message":"denied"}`,
		`{"status":"ok","retcode":100,"message":"denied"}`,
		`not-json`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		o := NewOneBot()
		o.httpBase = srv.URL
		_, err := o.call("get_group_list", map[string]any{})
		srv.Close()
		if err == nil {
			t.Fatalf("response %q was accepted", body)
		}
	}
}

func TestOneBotCallRejectsHTTPFailureAndOversize(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   string
	}{
		{http.StatusBadGateway, "upstream", "http 502"},
		{http.StatusOK, strings.Repeat("x", maxOneBotResponseBytes+1), "response too large"},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			fmt.Fprint(w, tc.body)
		}))
		o := NewOneBot()
		o.httpBase = srv.URL
		_, err := o.call("get_group_list", map[string]any{})
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("status=%d error=%v want=%q", tc.status, err, tc.want)
		}
	}
}

func TestOneBotHistoryHonorsContextCancellation(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	defer srv.Close()
	o := NewOneBot()
	o.httpBase = srv.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := o.GetGroupMsgHistoryContext(ctx, 42, 10, 0)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("history request did not start")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled history returned nil error")
		}
	case <-time.After(time.Second):
		t.Fatal("history did not honor cancellation")
	}
}

func TestOneBotReconfigureKeepsNewConnectionState(t *testing.T) {
	makeWS := func(closed chan struct{}) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
			if err != nil {
				t.Error(err)
				return
			}
			defer conn.Close()
			defer close(closed)
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}))
	}
	oldClosed := make(chan struct{})
	newClosed := make(chan struct{})
	old := makeWS(oldClosed)
	defer old.Close()
	newServer := makeWS(newClosed)
	defer newServer.Close()
	o := NewOneBot()
	t.Cleanup(func() {
		o.genMu.Lock()
		if o.cancel != nil {
			o.cancel()
		}
		o.genMu.Unlock()
	})
	wsURL := func(httpURL string) string { return "ws" + strings.TrimPrefix(httpURL, "http") }
	waitConnected := func(want bool) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if o.Connected() == want {
				return
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatalf("connection did not become %v", want)
	}
	o.Reconfigure(OneBotConfig{WSURL: wsURL(old.URL)})
	waitConnected(true)
	o.Reconfigure(OneBotConfig{WSURL: wsURL(newServer.URL)})
	select {
	case <-oldClosed:
	case <-time.After(2 * time.Second):
		t.Fatal("old WebSocket was not closed")
	}
	waitConnected(true)
	select {
	case <-newClosed:
		t.Fatal("new WebSocket closed during reconfigure")
	default:
	}
}
