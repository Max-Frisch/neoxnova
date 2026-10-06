package models

import "time"

type CelestialType string

const (
	TypePlanet    CelestialType = "PLANET"
	TypeMoon      CelestialType = "MOON"
	TypeDebris    CelestialType = "DEBRIS_FIELD"
	TypeDeepSpace CelestialType = "DEEP_SPACE"
)

type MissionType string

const (
	MissionAttack      MissionType = "ATTACK"
	MissionTransport   MissionType = "TRANSPORT"
	MissionDeploy      MissionType = "DEPLOY"
	MissionHold        MissionType = "HOLD"
	MissionEspionage   MissionType = "ESPIONAGE"
	MissionColonize    MissionType = "COLONIZE"
	MissionRecycle     MissionType = "RECYCLE"
	MissionDestroyMoon MissionType = "DESTROY_MOON"
	MissionExpedition  MissionType = "EXPEDITION"
)

type FleetPhase string

const (
	PhaseOutbound  FleetPhase = "OUTBOUND"
	PhaseHolding   FleetPhase = "HOLDING"
	PhaseReturning FleetPhase = "RETURNING"
	PhaseResolved  FleetPhase = "RESOLVED"
	PhaseCancelled FleetPhase = "CANCELLED"
)

type Coordinates struct {
	Galaxy   int           `json:"galaxy" validate:"min=1,max=9"`
	System   int           `json:"system" validate:"min=1,max=499"`
	Position int           `json:"position" validate:"min=1,max=21"`
	Type     CelestialType `json:"type"`
}

type CargoManifest struct {
	Metal      int64 `json:"metal" validate:"min=0"`
	Crystal    int64 `json:"crystal" validate:"min=0"`
	Deuterium  int64 `json:"deuterium" validate:"min=0"`
	DarkMatter int64 `json:"dark_matter,omitempty" validate:"min=0"`
}

type PlanetOverviewResponse struct {
	ServerTime       time.Time             `json:"server_time"`
	ActivePlanetID   int64                 `json:"active_planet_id"`
	ActivePlanetName string                `json:"active_planet_name"`
	Coordinates      Coordinates           `json:"coordinates"`
	DiameterKm       int                   `json:"diameter_km"`
	FieldsUsed       int                   `json:"fields_used"`
	FieldsMax        int                   `json:"fields_max"`
	TempRange        [2]int                `json:"temp_range_celsius"`
	PlayerRank       PlayerRankData        `json:"player_rank"`
	Levels           PlayerLevelsData      `json:"levels"`
	Resources        RealtimeResourceState `json:"resources"`
	Colonies         []ColonySummary       `json:"colonies"`
	ActiveFleets     []FleetEventSummary   `json:"active_fleets"`
	ActiveResearch   *QueueItemSummary     `json:"active_research,omitempty"`
}

type PlayerRankData struct {
	Points       int64 `json:"points"`
	Position     int   `json:"position"`
	TotalPlayers int   `json:"total_players"`
}

type PlayerLevelsData struct {
	PeacefulLevel       int     `json:"peaceful_level"`
	PeacefulProgressPct float64 `json:"peaceful_progress_pct"`
	PeacefulNextSeconds int64   `json:"peaceful_next_seconds"`
	CombatLevel         int     `json:"combat_level"`
	CombatXP            int64   `json:"combat_xp"`
	CombatXPNeeded      int64   `json:"combat_xp_needed"`
}

type RealtimeResourceState struct {
	MetalCurrent      float64   `json:"metal_current"`
	MetalLimit        int64     `json:"metal_limit"`
	MetalHourlyRate   float64   `json:"metal_hourly_rate"`
	CrystalCurrent    float64   `json:"crystal_current"`
	CrystalLimit      int64     `json:"crystal_limit"`
	CrystalHourlyRate float64   `json:"crystal_hourly_rate"`
	DeutCurrent       float64   `json:"deut_current"`
	DeutLimit         int64     `json:"deut_limit"`
	DeutHourlyRate    float64   `json:"deut_hourly_rate"`
	EnergyAvailable   int       `json:"energy_available"`
	EnergyMax         int       `json:"energy_max"`
	DarkMatter        int64     `json:"dark_matter"`
	Antimatter        int64     `json:"antimatter"`
	LastCalculatedAt  time.Time `json:"last_calculated_at"`
}

type ColonySummary struct {
	ID                  int64       `json:"id"`
	Name                string      `json:"name"`
	Coordinates         Coordinates `json:"coordinates"`
	IsUnderAttack       bool        `json:"is_under_attack"`
	IsBeingRaided       bool        `json:"is_being_raided"`
	HasMissilesInFlight bool        `json:"has_missiles_in_flight"`
}

type FleetEventSummary struct {
	FleetID       int64            `json:"fleet_id"`
	Mission       MissionType      `json:"mission"`
	Phase         FleetPhase       `json:"phase"`
	Origin        Coordinates      `json:"origin"`
	Destination   Coordinates      `json:"destination"`
	DepartureTime time.Time        `json:"departure_time"`
	ArrivalTime   time.Time        `json:"arrival_time"`
	RemainingSecs int64            `json:"remaining_seconds"`
	ShipManifest  map[string]int64 `json:"ship_manifest"`
	FleetPoints   int64            `json:"fleet_points"`
	Cargo         CargoManifest    `json:"cargo"`
}

type QueueItemSummary struct {
	ItemCode      string    `json:"item_code"`
	TargetLevel   int       `json:"target_level"`
	EndTime       time.Time `json:"end_time"`
	RemainingSecs int64     `json:"remaining_seconds"`
}

type FleetDispatchRequest struct {
	OriginPlanetID int64            `json:"origin_planet_id" validate:"required"`
	Target         Coordinates      `json:"target" validate:"required"`
	Mission        MissionType      `json:"mission" validate:"required"`
	Ships          map[string]int64 `json:"ships" validate:"required,min=1"`
	SpeedPercent   int              `json:"speed_percent" validate:"required,min=10,max=100"`
	Cargo          CargoManifest    `json:"cargo"`
	HoldingHours   int              `json:"holding_hours,omitempty" validate:"min=0,max=24"`
}

type FleetRecallRequest struct {
	FleetID int64 `json:"fleet_id" validate:"required"`
}

type StructureBuildRequest struct {
	StructureCode string `json:"structure_code"`
}

type ShipyardBuildRequest struct {
	UnitCode string `json:"unit_code"`
	Quantity int64  `json:"quantity"`
}

type ResearchRequest struct {
	TechCode string `json:"tech_code"`
}

// PlanetRelocateRequest is the body of POST /api/v1/planets/{id}/relocate.
type PlanetRelocateRequest struct {
	Galaxy   int `json:"galaxy" validate:"min=1,max=9"`
	System   int `json:"system" validate:"min=1,max=499"`
	Position int `json:"position" validate:"min=1,max=21"`
}

// PlanetRelocateResponse echoes the destination and the Dark Matter spent.
type PlanetRelocateResponse struct {
	Status      string      `json:"status"`
	Cost        int64       `json:"cost"`
	Coordinates Coordinates `json:"coordinates"`
}

// PlanetExpandFieldsRequest is the body of POST /api/v1/planets/{id}/fields.
type PlanetExpandFieldsRequest struct {
	Fields int `json:"fields" validate:"min=1,max=100"`
}

// PlanetExpandFieldsResponse reports the Dark Matter spent and the new totals.
type PlanetExpandFieldsResponse struct {
	Status       string `json:"status"`
	Cost         int64  `json:"cost"`
	FieldsMax    int64  `json:"fields_max"`
	FieldsBought int64  `json:"fields_bought"`
}

type QueueEntrySummary struct {
	ID            int64     `json:"id"`
	Code          string    `json:"code"`
	TargetLevel   int       `json:"target_level,omitempty"`
	Quantity      int64     `json:"quantity,omitempty"`
	StartTime     time.Time `json:"start_time"`
	EndTime       time.Time `json:"end_time"`
	RemainingSecs int64     `json:"remaining_seconds"`
}

type BuildingsResponse struct {
	PlanetID  int64                    `json:"planet_id"`
	Levels    map[string]int           `json:"levels"`
	Queue     *QueueEntrySummary       `json:"queue,omitempty"`
	NextCosts map[string]CargoManifest `json:"next_costs,omitempty"`
}
