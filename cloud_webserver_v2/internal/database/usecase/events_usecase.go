package usecase

import (
	"context"

	"github.com/hytech-racing/cloud-webserver-v2/internal/database/repository"
	"github.com/hytech-racing/cloud-webserver-v2/internal/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

type EventsUseCase struct {
	EventsRepo repository.EventsRepository
}

func NewEventsUseCase(eventsRepo repository.EventsRepository) *EventsUseCase {
	return &EventsUseCase{
		EventsRepo: eventsRepo,
	}
}

func (uc *EventsUseCase) CreateEvents(ctx context.Context, model *models.EventsModel) (*models.EventsModel, error) {
	model, err := uc.EventsRepo.Save(ctx, model)
	if err != nil {
		return nil, err
	}
	return model, nil
}

func (uc *EventsUseCase) GetEventsByFilters(ctx context.Context, filters *models.EventsModelFilters) ([]models.EventsModel, error) {
	bson_filters_m := bson.M{}
	bson_or := bson.A{}

	if filters.ID != nil {
		id, err := primitive.ObjectIDFromHex(filters.ID.Hex())
		if err != nil {
			return nil, err
		}
		bson_filters_m["id"] = id
	}

	if filters.BeforeDate != nil || filters.AfterDate != nil {
		dateFilter := bson.M{}
		if filters.BeforeDate != nil {
			dateFilter["$gte"] = *filters.BeforeDate
		}
		if filters.AfterDate != nil {
			dateFilter["$lte"] = *filters.AfterDate
		}
		bson_filters_m["date"] = dateFilter
	}

	if filters.EventType != nil {
		bson_filters_m["event_type"] = bson.M{"$regex": primitive.Regex{Pattern: *filters.EventType, Options: "i"}}
	}

	if len(bson_or) != 0 {
		bson_filters_m["$or"] = bson_or
	}

	result, err := uc.EventsRepo.GetWithEventsFilters(context.TODO(), &bson_filters_m)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (uc *EventsUseCase) GetEventsById(ctx context.Context, id primitive.ObjectID) (*models.EventsModel, error) {
	return uc.EventsRepo.GetEventsFromId(ctx, id)
}

func (uc *EventsUseCase) DeleteEventsById(ctx context.Context, id primitive.ObjectID) error {
	return uc.EventsRepo.DeleteEventsFromId(ctx, id)
}

func (uc *EventsUseCase) UpdateEventsRun(ctx context.Context, id primitive.ObjectID, model *models.EventsModel) error {
	return uc.EventsRepo.UpdateEventsFromId(ctx, id, model)
}
func (uc *EventsUseCase) LogEvent(ctx context.Context, mcapID primitive.ObjectID, filename string, event string, errMsg *string) (*models.EventsModel, error) {
	now := time.Now()
	model := &models.EventsModel{
		McapID:       &mcapID,
		Event:        &event,
		Error:        errMsg,
		CreatedAt:    &now,
	}
	return uc.CreateEvents(ctx, model)
}
