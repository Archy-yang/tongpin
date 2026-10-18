package icebreak

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/Archy-yang/tongpin/backend/internal/analytics"
	"github.com/Archy-yang/tongpin/backend/internal/llm"
)

var (
	ErrNotParticipant = errors.New("not a participant of this match")
	ErrWrongState     = errors.New("match is not in a state for this action")
	ErrNotRequestParty = errors.New("not a party of this icebreak session")
	ErrSessionExists  = errors.New("icebreak session already exists")
)

// MatchGateway 本域对 match 域的最小依赖（消费方接口，装配层注入）。
type MatchGateway interface {
	ParticipantOf(ctx context.Context, matchID, userID int64) (bool, error)
	CounterpartOf(ctx context.Context, matchID, userID int64) (int64, error)
	StatusOf(ctx context.Context, matchID int64) (string, error)
	SetStatus(ctx context.Context, matchID int64, status string) error
}

// ConsentRecorder 授权审计。
type ConsentRecorder interface {
	Record(ctx context.Context, userID int64, targetType string, targetID *int64, granted bool) error
}

// Completer LLM 能力（system + user 两段式，与 style 域同型）。
type Completer interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// ProfileSource 风格画像数据源（style 域适配）。
type ProfileSource interface {
	LatestAnalysis(ctx context.Context, userID int64) (profile []byte, confidence float64, ok bool, err error)
}

const agentRounds = 3 // 双代理各说 3 次

type Service struct {
	repo     *Repository
	match    MatchGateway
	consents ConsentRecorder
	profiles ProfileSource
	completer Completer
	rec      *analytics.Recorder // 可为 nil（不埋点）
}

// SetRecorder 装配期注入埋点。
func (s *Service) SetRecorder(r *analytics.Recorder) { s.rec = r }

func NewService(repo *Repository, match MatchGateway, consents ConsentRecorder, profiles ProfileSource, completer Completer) *Service {
	return &Service{repo: repo, match: match, consents: consents, profiles: profiles, completer: completer}
}

// Create 发起暖场请求。match 需处于 suggested（icebreak_ready 后直接看摘要即可）。
func (s *Service) Create(ctx context.Context, userID, matchID int64) (Session, error) {
	if ok, err := s.match.ParticipantOf(ctx, matchID, userID); err != nil || !ok {
		return Session{}, ErrNotParticipant
	}
	if s.rec != nil {
		s.rec.Record(ctx, userID, analytics.EvIcebreakCreate, nil)
	}
	status, err := s.match.StatusOf(ctx, matchID)
	if err != nil {
		return Session{}, err
	}
	if status != "suggested" {
		return Session{}, ErrWrongState
	}
	if existing, err := s.repo.GetByMatch(ctx, matchID); err != nil {
		return Session{}, err
	} else if existing != nil {
		return Session{}, ErrSessionExists
	}

	counterpart, err := s.match.CounterpartOf(ctx, matchID, userID)
	if err != nil {
		return Session{}, err
	}
	sess, err := s.repo.Create(ctx, matchID, userID, counterpart)
	if err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	if err := s.match.SetStatus(ctx, matchID, "icebreak_requested"); err != nil {
		return Session{}, fmt.Errorf("update match: %w", err)
	}
	if err := s.consents.Record(ctx, userID, "ai_icebreak", &matchID, true); err != nil {
		return Session{}, fmt.Errorf("record consent: %w", err)
	}
	return sess, nil
}

// Approve 被请求方同意 → 异步生成摘要（LLM 多次调用，同步会超时）。
func (s *Service) Approve(ctx context.Context, userID, sessionID int64) error {
	sess, err := s.getByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if sess == nil || (sess.FromUser != userID && sess.ToUser != userID) {
		return ErrNotRequestParty
	}
	if sess.ToUser != userID {
		return ErrNotRequestParty // 只有被请求方（同意方）能 approve
	}
	if sess.Status != StatusPending {
		return ErrWrongState
	}

	if err := s.repo.UpdateStatus(ctx, sess.ID, StatusGenerating); err != nil {
		return err
	}
	if err := s.match.SetStatus(ctx, sess.MatchID, "icebreak_generating"); err != nil {
		return err
	}
	if err := s.consents.Record(ctx, userID, "ai_icebreak", &sess.MatchID, true); err != nil {
		return err
	}

	// 脱离请求上下文后台生成；HTTP 立即返回，前端轮询状态
	go s.generate(context.WithoutCancel(ctx), sess)
	return nil
}

// Decline 婉拒暖场 → 会话 declined，match 回到 suggested（直接真人接触的路径仍开放）。
func (s *Service) Decline(ctx context.Context, userID, sessionID int64) error {
	sess, err := s.getByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if sess == nil || sess.ToUser != userID {
		return ErrNotRequestParty
	}
	if sess.Status != StatusPending {
		return ErrWrongState
	}
	if err := s.repo.UpdateStatus(ctx, sess.ID, StatusDeclined); err != nil {
		return err
	}
	return s.match.SetStatus(ctx, sess.MatchID, "suggested")
}

// ForUser 我参与的全部暖场会话。
func (s *Service) ForUser(ctx context.Context, userID int64) ([]Session, error) {
	return s.repo.ForUser(ctx, userID)
}

// generate 双代理试聊 agentRounds 轮 + 摘要。后台 goroutine 调用，错误只落日志与状态。
func (s *Service) generate(ctx context.Context, sess *Session) {
	profileA, _, okA, err := s.profiles.LatestAnalysis(ctx, sess.FromUser)
	profileB, _, okB, err2 := s.profiles.LatestAnalysis(ctx, sess.ToUser)
	if err != nil || err2 != nil || !okA || !okB {
		log.Printf("[icebreak] session %d: profiles unavailable (%v/%v)", sess.ID, err, err2)
		_ = s.repo.UpdateStatus(ctx, sess.ID, StatusFailed)
		return
	}

	var transcript []Turn
	for round := 0; round < agentRounds; round++ {
		for _, side := range []string{"a", "b"} {
			profile := profileB
			attributed := sess.ToUser // b 代理的 token 归属对方
			if side == "a" {
				profile = profileA
				attributed = sess.FromUser
			}
			text, err := s.completer.Complete(llm.WithUser(ctx, attributed),
				agentSystemPrompt(string(profile), side),
				buildAgentTurn(side, flip(side), transcript))
			if err != nil {
				log.Printf("[icebreak] session %d: agent %s round %d: %v", sess.ID, side, round, err)
				_ = s.repo.UpdateStatus(ctx, sess.ID, StatusFailed)
				return
			}
			transcript = append(transcript, Turn{Side: side, Text: text})
		}
	}

	raw, err := s.completer.Complete(llm.WithUser(ctx, sess.FromUser), summarySystemPrompt, buildSummaryUser(profileA, profileB, transcript))
	if err != nil {
		log.Printf("[icebreak] session %d: summarize: %v", sess.ID, err)
		_ = s.repo.UpdateStatus(ctx, sess.ID, StatusFailed)
		return
	}
	summary, err := ParseSummary(raw)
	if err != nil {
		log.Printf("[icebreak] session %d: %v", sess.ID, err)
		_ = s.repo.UpdateStatus(ctx, sess.ID, StatusFailed)
		return
	}

	if err := s.repo.SaveResult(ctx, sess.ID, transcript, &summary); err != nil {
		log.Printf("[icebreak] session %d: save: %v", sess.ID, err)
		return
	}
	if err := s.match.SetStatus(ctx, sess.MatchID, "icebreak_ready"); err != nil {
		log.Printf("[icebreak] session %d: update match: %v", sess.ID, err)
		return
	}
	log.Printf("[icebreak] session %d ready: match %d", sess.ID, sess.MatchID)
}

func (s *Service) getByID(ctx context.Context, sessionID int64) (*Session, error) {
	return s.repo.GetByID(ctx, sessionID)
}

func flip(side string) string {
	if side == "a" {
		return "b"
	}
	return "a"
}
