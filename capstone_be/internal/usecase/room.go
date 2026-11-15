package usecase

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"math/big"

	"capstonewebserver/internal/domain"
	"capstonewebserver/internal/dto"
	"capstonewebserver/internal/reqctx"
)

// roomIDAlphabet excludes look-alikes (O/0, 1/I), per contracts.md.
const roomIDAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// RoomConfig is RoomUseCase's static configuration, normally sourced from
// .env.
type RoomConfig struct {
	Secret     []byte
	Passcode   string
	SfuWsURL   string
	ICEServers []dto.ICEServer
}

// RoomUseCase implements handler.RoomUseCase.
type RoomUseCase struct {
	rs        RoomStore
	client    RequestClient
	blacklist BlacklistClearer
	events    EventPublisher
	cfg       RoomConfig
}

func NewRoomUseCase(rs RoomStore, client RequestClient, blacklist BlacklistClearer, events EventPublisher, cfg RoomConfig) *RoomUseCase {
	return &RoomUseCase{rs: rs, client: client, blacklist: blacklist, events: events, cfg: cfg}
}

func (uc *RoomUseCase) CreateRoom(ctx context.Context, passcode string) (dto.RoomCreationResponse, error) {
	if subtle.ConstantTimeCompare([]byte(passcode), []byte(uc.cfg.Passcode)) != 1 {
		return dto.RoomCreationResponse{}, domain.ErrInvalidPasscode
	}

	var creatorIP string
	if ip, ok := reqctx.GetIP(ctx); ok && ip != nil {
		creatorIP = *ip
	}

	roomId, err := newRoomID()
	if err != nil {
		return dto.RoomCreationResponse{}, err
	}

	if err := uc.rs.Open(ctx, roomId, creatorIP); err != nil {
		return dto.RoomCreationResponse{}, err
	}

	token, err := SignAdminToken(uc.cfg.Secret, roomId)
	if err != nil {
		return dto.RoomCreationResponse{}, err
	}

	return dto.RoomCreationResponse{RoomID: roomId, AdminToken: token}, nil
}

func (uc *RoomUseCase) RoomDetail(ctx context.Context, roomId string) (dto.RoomQueryResponse, error) {
	exist, err := uc.rs.IsExist(ctx, roomId)
	if err != nil {
		return dto.RoomQueryResponse{}, fmt.Errorf("room store: %w", err)
	}
	if !exist {
		return dto.RoomQueryResponse{}, domain.ErrRoomNotFound
	}

	publishers, err := uc.client.GetPublishers(ctx, roomId)
	if err != nil {
		return dto.RoomQueryResponse{}, fmt.Errorf("%w: %v", domain.ErrSFUUnreachable, err)
	}
	if publishers == nil {
		publishers = []dto.Publisher{}
	}

	return dto.RoomQueryResponse{
		RoomID:     roomId,
		Publishers: publishers,
		SfuWsURL:   uc.cfg.SfuWsURL,
		ICEServers: uc.cfg.ICEServers,
	}, nil
}

func (uc *RoomUseCase) RoomPublishers(ctx context.Context, roomId string) (dto.RoomPublisherQueryResponse, error) {
	exist, err := uc.rs.IsExist(ctx, roomId)
	if err != nil {
		return dto.RoomPublisherQueryResponse{}, fmt.Errorf("room store: %w", err)
	}
	if !exist {
		return dto.RoomPublisherQueryResponse{}, domain.ErrRoomNotFound
	}

	publishers, err := uc.client.GetPublishers(ctx, roomId)
	if err != nil {
		return dto.RoomPublisherQueryResponse{}, fmt.Errorf("%w: %v", domain.ErrSFUUnreachable, err)
	}
	if publishers == nil {
		publishers = []dto.Publisher{}
	}

	return dto.RoomPublisherQueryResponse{RoomID: roomId, Publishers: publishers}, nil
}

func (uc *RoomUseCase) ReclaimRoom(ctx context.Context, roomId string, ip string) (dto.RoomReclaimResponse, error) {
	ownerIp, err := uc.rs.OwnerIp(ctx, roomId)
	if err != nil {
		return dto.RoomReclaimResponse{}, err
	}
	if ownerIp != ip {
		return dto.RoomReclaimResponse{}, domain.ErrIPMismatch
	}

	token, err := SignAdminToken(uc.cfg.Secret, roomId)
	if err != nil {
		return dto.RoomReclaimResponse{}, err
	}

	return dto.RoomReclaimResponse{RoomID: roomId, AdminToken: token}, nil
}

func (uc *RoomUseCase) CloseRoom(ctx context.Context, roomId string) error {
	exist, err := uc.rs.IsExist(ctx, roomId)
	if err != nil {
		return fmt.Errorf("room store: %w", err)
	}
	if !exist {
		return domain.ErrRoomNotFound
	}

	if err := uc.client.CloseRoom(ctx, roomId); err != nil {
		return fmt.Errorf("%w: %v", domain.ErrSFUUnreachable, err)
	}

	if err := uc.rs.Close(ctx, roomId); err != nil {
		return fmt.Errorf("room store: %w", err)
	}
	if err := uc.blacklist.Clear(ctx); err != nil {
		return fmt.Errorf("blacklist store: %w", err)
	}

	uc.events.Publish(ctx, roomId, domain.Event{Name: domain.EventRoomClosed, Data: dto.RoomClosedEvent{RoomID: roomId}})
	return nil
}

// newRoomID generates a 3+3 character room id from roomIDAlphabet
// (contracts.md: "3 + 3 characters from A-Z and 2-9", "no look-alikes").
func newRoomID() (string, error) {
	b := make([]byte, 6)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(roomIDAlphabet))))
		if err != nil {
			return "", err
		}
		b[i] = roomIDAlphabet[n.Int64()]
	}
	return string(b[:3]) + "-" + string(b[3:]), nil
}
