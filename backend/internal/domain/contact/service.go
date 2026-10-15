package contact

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrNotParticipant = errors.New("not a participant of this match")
	ErrWrongState     = errors.New("match is not in a state for this action")
	ErrNotRequestParty = errors.New("not a party of this request")
	ErrChatNotOpen    = errors.New("real chat is not open for this match")
	ErrBlocked        = errors.New("blocked")
)

// MatchGateway 本域对 match 域的最小依赖（消费方接口，装配层注入）。
type MatchGateway interface {
	ParticipantOf(ctx context.Context, matchID, userID int64) (bool, error)
	CounterpartOf(ctx context.Context, matchID, userID int64) (int64, error)
	StatusOf(ctx context.Context, matchID int64) (string, error)
	SetStatus(ctx context.Context, matchID int64, status string) error
}

type BlockChecker interface {
	BlockedBetween(ctx context.Context, a, b int64) (bool, error)
}

type ConsentRecorder interface {
	Record(ctx context.Context, userID int64, targetType string, targetID *int64, granted bool) error
}

// UserDirectory 昵称展示所需的最小依赖。
type UserDirectory interface {
	Nickname(ctx context.Context, userID int64) (string, error)
}

// RealtimePublisher 新消息实时推送（realtime Hub 适配，可为 nil）。
type RealtimePublisher interface {
	PublishDirectMessage(ctx context.Context, matchID int64, msg DirectMessage, toUsers ...int64)
}

type Service struct {
	repo     *Repository
	match    MatchGateway
	blocks   BlockChecker
	consents ConsentRecorder
	users    UserDirectory
	notifier RealtimePublisher
}

func NewService(repo *Repository, match MatchGateway, blocks BlockChecker, consents ConsentRecorder, users UserDirectory) *Service {
	return &Service{repo: repo, match: match, blocks: blocks, consents: consents, users: users}
}

// Create 发起真人接触请求（请求方）。需：我是 match 参与方，match 处于
// suggested（直接接触）或 icebreak_ready（看过暖场摘要后决定见面）。
func (s *Service) Create(ctx context.Context, userID, matchID int64) (Request, error) {
	if ok, err := s.match.ParticipantOf(ctx, matchID, userID); err != nil || !ok {
		return Request{}, ErrNotParticipant
	}
	status, err := s.match.StatusOf(ctx, matchID)
	if err != nil {
		return Request{}, err
	}
	if status != "suggested" && status != "icebreak_ready" {
		return Request{}, ErrWrongState
	}

	// 对方 ID 从 match 参与方推出
	counterpart, err := s.counterpartOf(ctx, matchID, userID)
	if err != nil {
		return Request{}, err
	}

	req, err := s.repo.InsertRequest(ctx, matchID, userID, counterpart)
	if err != nil {
		return Request{}, fmt.Errorf("insert request: %w", err)
	}
	if err := s.match.SetStatus(ctx, matchID, "contact_requested"); err != nil {
		return Request{}, fmt.Errorf("update match status: %w", err)
	}
	if err := s.consents.Record(ctx, userID, "real_contact", &matchID, true); err != nil {
		return Request{}, fmt.Errorf("record consent: %w", err)
	}
	return req, nil
}

// Approve 同意接触（被请求方）→ 双方进入真人聊天。
func (s *Service) Approve(ctx context.Context, userID, requestID int64) error {
	req, err := s.repo.GetRequest(ctx, requestID)
	if err != nil {
		return err
	}
	if req == nil || req.ToUser != userID {
		return ErrNotRequestParty
	}
	if req.Status != "pending" {
		return ErrWrongState
	}
	if err := s.repo.UpdateRequestStatus(ctx, requestID, "approved"); err != nil {
		return err
	}
	if err := s.match.SetStatus(ctx, req.MatchID, "in_chat"); err != nil {
		return err
	}
	return s.consents.Record(ctx, userID, "real_contact", &req.MatchID, true)
}

// Reject 拒绝（静默，不通知请求方）→ 匹配关闭。
func (s *Service) Reject(ctx context.Context, userID, requestID int64) error {
	req, err := s.repo.GetRequest(ctx, requestID)
	if err != nil {
		return err
	}
	if req == nil || req.ToUser != userID {
		return ErrNotRequestParty
	}
	if req.Status != "pending" {
		return ErrWrongState
	}
	if err := s.repo.UpdateRequestStatus(ctx, requestID, "rejected"); err != nil {
		return err
	}
	return s.match.SetStatus(ctx, req.MatchID, "closed")
}

// Revoke 请求方撤回未决请求 → 匹配回到 suggested。
func (s *Service) Revoke(ctx context.Context, userID, requestID int64) error {
	req, err := s.repo.GetRequest(ctx, requestID)
	if err != nil {
		return err
	}
	if req == nil || req.FromUser != userID {
		return ErrNotRequestParty
	}
	if req.Status != "pending" {
		return ErrWrongState
	}
	if err := s.repo.UpdateRequestStatus(ctx, requestID, "revoked"); err != nil {
		return err
	}
	return s.match.SetStatus(ctx, req.MatchID, "suggested")
}

// List 收件箱 / 发件箱。
func (s *Service) List(ctx context.Context, userID int64, inbox bool) ([]RequestView, error) {
	reqs, err := s.repo.ListRequests(ctx, userID, inbox, true)
	if err != nil {
		return nil, err
	}
	out := make([]RequestView, 0, len(reqs))
	for _, req := range reqs {
		view := RequestView{Request: req}
		other := req.ToUser
		if inbox {
			other = req.FromUser
		}
		if nickname, err := s.users.Nickname(ctx, other); err == nil {
			view.CounterpartNickname = nickname
		}
		out = append(out, view)
	}
	return out, nil
}

// SendDirect 真人一对一发消息。仅双方 approved（match = in_chat）后可用。
// 落库后尽力经 WebSocket 推送双方（在线即秒达；离线靠拉取兜底）。
func (s *Service) SendDirect(ctx context.Context, userID, matchID int64, content string) (DirectMessage, error) {
	if err := s.ensureChatOpen(ctx, userID, matchID); err != nil {
		return DirectMessage{}, err
	}
	msg, err := s.repo.InsertDirectMessage(ctx, matchID, userID, content)
	if err != nil {
		return DirectMessage{}, err
	}
	if s.notifier != nil {
		counterpart, err := s.counterpartOf(ctx, matchID, userID)
		if err == nil {
			s.notifier.PublishDirectMessage(ctx, matchID, msg, userID, counterpart)
		}
	}
	return msg, nil
}

// ListDirect 真人聊天历史（游标分页）。
func (s *Service) ListDirect(ctx context.Context, userID, matchID, before int64, limit int) ([]DirectMessage, error) {
	if err := s.ensureChatOpen(ctx, userID, matchID); err != nil {
		return nil, err
	}
	msgs, err := s.repo.ListDirectMessages(ctx, matchID, before, limit)
	if err != nil {
		return nil, err
	}
	for i := range msgs {
		msgs[i].IsMine = msgs[i].SenderID == userID
	}
	return msgs, nil
}

// SetNotifier 装配期注入实时推送（一次性，先于任何请求）。
func (s *Service) SetNotifier(n RealtimePublisher) {
	s.notifier = n
}

// ensureChatOpen 参与方 + match 状态 in_chat + 未互相拉黑。
func (s *Service) ensureChatOpen(ctx context.Context, userID, matchID int64) error {
	if ok, err := s.match.ParticipantOf(ctx, matchID, userID); err != nil || !ok {
		return ErrNotParticipant
	}
	status, err := s.match.StatusOf(ctx, matchID)
	if err != nil {
		return err
	}
	if status != "in_chat" {
		return ErrChatNotOpen
	}
	counterpart, err := s.counterpartOf(ctx, matchID, userID)
	if err != nil {
		return err
	}
	blocked, err := s.blocks.BlockedBetween(ctx, userID, counterpart)
	if err != nil {
		return err
	}
	if blocked {
		return ErrBlocked
	}
	return nil
}

// counterpartOf 找到 match 中另一个参与者的 ID。
func (s *Service) counterpartOf(ctx context.Context, matchID, userID int64) (int64, error) {
	return s.match.CounterpartOf(ctx, matchID, userID)
}
