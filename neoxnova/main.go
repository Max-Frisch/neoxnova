package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"neoxnova/engine"
	"neoxnova/models"
	"neoxnova/utils"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

func main() {
	log.Println("[INIT] Launching Neo-XNova Game Engine v2.0...")

	// 1. Establish Context for safe application shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 2. Initialize PostgreSQL Connection Pool
	pgConnString := "postgres://postgres:password@localhost:5432/neoxnova?sslmode=disable"
	db, err := sql.Open("postgres", pgConnString)
	if err != nil {
		log.Fatalf("[FATAL] Failed to configure PostgreSQL driver: %v", err)
	}
	defer db.Close()

	// Configure pool boundaries to optimize concurrency
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// 3. Initialize Redis Connection Client
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // Default empty for local docker setup
		DB:       0,
	})
	defer rdb.Close()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("[WARNING] Redis connection test failed. Is Redis running? Error: %v", err)
	} else {
		log.Println("[INIT] Connected to Redis Event Wheel successfully.")
	}

	// 4. Spin up the Background Fleet Time-Wheel Daemon Loop
	universeID := "universe_6_niburu"
	eventEngine := engine.NewEventEngine(rdb, db)
	go eventEngine.StartEventLoop(ctx, universeID)

	// 5. Define HTTP REST Routing Endpoints (Go 1.22+ Native Syntax)
	mux := http.NewServeMux()

	// Health Check / Verification endpoint
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"online","engine":"Neo-XNova v2.0","universe":"`+universeID+`"}`)
	})

	// Real-Time Calculated Planet Resources Endpoint (Zero-Cron Model)
	mux.HandleFunc("GET /api/v1/planets/{id}/resources", func(w http.ResponseWriter, r *http.Request) {
		planetID := r.PathValue("id")
		w.Header().Set("Content-Type", "application/json")

		var outMetal, outCrystal, outDeut float64
		var outLastCalc time.Time
		err := db.QueryRowContext(r.Context(), "SELECT * FROM update_celestial_resources($1)", planetID).Scan(
			&outMetal, &outCrystal, &outDeut, &outLastCalc,
		)
		if err != nil {
			log.Printf("[ERROR] Resource accumulation failed for planet %s: %v", planetID, err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Failed to calculate continuous resources"})
			return
		}

		var metalCap, crystalCap, deutCap int64
		var metalRate, crystalRate, deutRate float64
		var energyAvail, energyMax int
		query := `SELECT metal_capacity, crystal_capacity, deuterium_capacity,
		                 metal_prod_hourly, crystal_prod_hourly, deuterium_prod_hourly,
		                 energy_used, energy_max
		          FROM celestial_objects WHERE id = $1`
		err = db.QueryRowContext(r.Context(), query, planetID).Scan(
			&metalCap, &crystalCap, &deutCap,
			&metalRate, &crystalRate, &deutRate,
			&energyAvail, &energyMax,
		)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Planet not found"})
			return
		}

		resp := models.RealtimeResourceState{
			MetalCurrent:      outMetal,
			MetalLimit:        metalCap,
			MetalHourlyRate:   metalRate,
			CrystalCurrent:    outCrystal,
			CrystalLimit:      crystalCap,
			CrystalHourlyRate: crystalRate,
			DeutCurrent:       outDeut,
			DeutLimit:         deutCap,
			DeutHourlyRate:    deutRate,
			EnergyAvailable:   energyAvail,
			EnergyMax:         energyMax,
			LastCalculatedAt:  outLastCalc,
		}
		json.NewEncoder(w).Encode(resp)
	})

	// Full Planet Overview Endpoint (replaces game.php?page=overview)
	mux.HandleFunc("GET /api/v1/planets/{id}/overview", func(w http.ResponseWriter, r *http.Request) {
		planetID := r.PathValue("id")
		w.Header().Set("Content-Type", "application/json")

		// 1. Calculate and update continuous resources
		var outMetal, outCrystal, outDeut float64
		var outLastCalc time.Time
		err := db.QueryRowContext(r.Context(), "SELECT * FROM update_celestial_resources($1)", planetID).Scan(
			&outMetal, &outCrystal, &outDeut, &outLastCalc,
		)
		if err != nil {
			log.Printf("[ERROR] Resource calculation failed: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Failed to update resources"})
			return
		}

		// 2. Fetch planet structural metadata
		var planetName string
		var galaxy, system, position, diameter, fieldsUsed, fieldsMax, tempMin, tempMax int
		var metalCap, crystalCap, deutCap int64
		var metalRate, crystalRate, deutRate float64
		var energyUsed, energyMax int
		var userID int64

		query := `SELECT user_id, name, galaxy, system, position, diameter_km, 
		                 fields_used, fields_max, temp_min, temp_max,
		                 metal_capacity, crystal_capacity, deuterium_capacity,
		                 metal_prod_hourly, crystal_prod_hourly, deuterium_prod_hourly,
		                 energy_used, energy_max
		          FROM celestial_objects WHERE id = $1`
		err = db.QueryRowContext(r.Context(), query, planetID).Scan(
			&userID, &planetName, &galaxy, &system, &position, &diameter,
			&fieldsUsed, &fieldsMax, &tempMin, &tempMax,
			&metalCap, &crystalCap, &deutCap,
			&metalRate, &crystalRate, &deutRate,
			&energyUsed, &energyMax,
		)
		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Planet not found"})
			return
		} else if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// 3. Fetch active fleets in flight for this player
		fleetRows, err := db.QueryContext(r.Context(), `
			SELECT id, mission, phase, origin_galaxy, origin_system, origin_position, origin_type,
			       target_galaxy, target_system, target_position, target_type,
			       start_time, arrival_time, cargo_metal, cargo_crystal, cargo_deuterium
			FROM fleets
			WHERE user_id = $1 AND phase IN ('OUTBOUND', 'HOLDING', 'RETURNING')
			ORDER BY arrival_time ASC
		`, userID)

		activeFleets := make([]models.FleetEventSummary, 0)
		if err == nil {
			defer fleetRows.Close()
			for fleetRows.Next() {
				var f models.FleetEventSummary
				var oG, oS, oP, tG, tS, tP int
				var oT, tT, missionStr, phaseStr string
				err := fleetRows.Scan(
					&f.FleetID, &missionStr, &phaseStr,
					&oG, &oS, &oP, &oT,
					&tG, &tS, &tP, &tT,
					&f.DepartureTime, &f.ArrivalTime,
					&f.Cargo.Metal, &f.Cargo.Crystal, &f.Cargo.Deuterium,
				)
				if err == nil {
					f.Mission = models.MissionType(missionStr)
					f.Phase = models.FleetPhase(phaseStr)
					f.Origin = models.Coordinates{Galaxy: oG, System: oS, Position: oP, Type: models.CelestialType(oT)}
					f.Destination = models.Coordinates{Galaxy: tG, System: tS, Position: tP, Type: models.CelestialType(tT)}
					f.RemainingSecs = int64(time.Until(f.ArrivalTime).Seconds())
					if f.RemainingSecs < 0 {
						f.RemainingSecs = 0
					}
					activeFleets = append(activeFleets, f)
				}
			}
		}

		pID, _ := strconv.ParseInt(planetID, 10, 64)
		resp := models.PlanetOverviewResponse{
			ServerTime:       time.Now(),
			ActivePlanetID:   pID,
			ActivePlanetName: planetName,
			Coordinates: models.Coordinates{
				Galaxy:   galaxy,
				System:   system,
				Position: position,
				Type:     models.TypePlanet,
			},
			DiameterKm: diameter,
			FieldsUsed: fieldsUsed,
			FieldsMax:  fieldsMax,
			TempRange:  [2]int{tempMin, tempMax},
			Resources: models.RealtimeResourceState{
				MetalCurrent:      outMetal,
				MetalLimit:        metalCap,
				MetalHourlyRate:   metalRate,
				CrystalCurrent:    outCrystal,
				CrystalLimit:      crystalCap,
				CrystalHourlyRate: crystalRate,
				DeutCurrent:       outDeut,
				DeutLimit:         deutCap,
				DeutHourlyRate:    deutRate,
				EnergyAvailable:   energyUsed,
				EnergyMax:         energyMax,
				LastCalculatedAt:  outLastCalc,
			},
			ActiveFleets: activeFleets,
		}
		json.NewEncoder(w).Encode(resp)
	})

	// Fleet Dispatch Route (replaces legacy multi-step fleetTable form submit)
	mux.HandleFunc("POST /api/v1/fleets/dispatch", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var req models.FleetDispatchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request payload"})
			return
		}

		// 1. Begin atomic transaction
		tx, err := db.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		// 2. Fetch origin planet coordinates, universe, and user with row lock
		var oG, oS, oP int
		var uID UUID
		var userID int64
		var originDeut float64
		originQuery := `SELECT universe_id, user_id, galaxy, system, position, deuterium 
		                FROM celestial_objects WHERE id = $1 FOR UPDATE`
		err = tx.QueryRowContext(r.Context(), originQuery, req.OriginPlanetID).Scan(
			&uID, &userID, &oG, &oS, &oP, &originDeut,
		)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Origin planet not found"})
			return
		}

		// 3. Verify ships on origin planet and deduct atomically
		for shipCode, count := range req.Ships {
			var available int64
			err := tx.QueryRowContext(r.Context(), `
				SELECT quantity FROM planet_ships WHERE celestial_id = $1 AND ship_code = $2 FOR UPDATE
			`, req.OriginPlanetID, shipCode).Scan(&available)

			if err != nil || available < count {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Insufficient ships for code %s (available: %d, requested: %d)", shipCode, available, count)})
				return
			}

			// Deduct ships from planet hangar
			_, err = tx.ExecContext(r.Context(), `
				UPDATE planet_ships SET quantity = quantity - $1 
				WHERE celestial_id = $2 AND ship_code = $3
			`, count, req.OriginPlanetID, shipCode)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}

		// 4. Calculate Distance, Duration, and Fuel Consumption using game_math
		dist := utils.CalculateCoordinateDistance(oG, oS, oP, req.Target.Galaxy, req.Target.System, req.Target.Position)
		baseSpeed := 10000 // default battleship baseline
		durationSecs := utils.CalculateFlightDuration(dist, baseSpeed, req.SpeedPercent)

		// Base fuel burn calculation (approx 500 deut per ship)
		var totalShipCount int64
		for _, c := range req.Ships {
			totalShipCount += c
		}
		fuelBurn := utils.CalculateDeuteriumConsumption(totalShipCount, 500, dist)

		if originDeut < float64(fuelBurn) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Insufficient deuterium for flight fuel (needed: %d)", fuelBurn)})
			return
		}

		// Deduct fuel burn immediately from origin planet
		_, err = tx.ExecContext(r.Context(), `
			UPDATE celestial_objects SET deuterium = deuterium - $1 WHERE id = $2
		`, fuelBurn, req.OriginPlanetID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// 5. Insert new fleet mission record
		now := time.Now()
		arrivalTime := now.Add(time.Duration(durationSecs) * time.Second)
		var holdingEnd sql.NullTime
		if req.Mission == models.MissionExpedition && req.HoldingHours > 0 {
			holdingEnd = sql.NullTime{Time: arrivalTime.Add(time.Duration(req.HoldingHours) * time.Hour), Valid: true}
		}
		returnTime := arrivalTime.Add(time.Duration(durationSecs) * time.Second)
		if holdingEnd.Valid {
			returnTime = holdingEnd.Time.Add(time.Duration(durationSecs) * time.Second)
		}

		var fleetID int64
		insertQuery := `
			INSERT INTO fleets (
				universe_id, user_id, mission, phase, origin_id, 
				origin_galaxy, origin_system, origin_position, origin_type,
				target_galaxy, target_system, target_position, target_type,
				start_time, arrival_time, holding_end_time, return_time,
				flight_speed_pct, deuterium_consumption,
				cargo_metal, cargo_crystal, cargo_deuterium
			) VALUES (
				$1, $2, $3, 'OUTBOUND', $4,
				$5, $6, $7, 'PLANET',
				$8, $9, $10, $11,
				$12, $13, $14, $15,
				$16, $17,
				$18, $19, $20
			) RETURNING id
		`
		err = tx.QueryRowContext(r.Context(), insertQuery,
			uID, userID, req.Mission, req.OriginPlanetID,
			oG, oS, oP,
			req.Target.Galaxy, req.Target.System, req.Target.Position, req.Target.Type,
			now, arrivalTime, holdingEnd, returnTime,
			float64(req.SpeedPercent)/100.0, fuelBurn,
			req.Cargo.Metal, req.Cargo.Crystal, req.Cargo.Deuterium,
		).Scan(&fleetID)
		if err != nil {
			log.Printf("[ERROR] Failed to insert fleet: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// 6. Record fleet ship manifest
		for shipCode, count := range req.Ships {
			_, err = tx.ExecContext(r.Context(), `
				INSERT INTO fleet_ships (fleet_id, ship_code, count) VALUES ($1, $2, $3)
			`, fleetID, shipCode, count)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}

		if err := tx.Commit(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// 7. Enqueue into Redis ZSET Event Wheel
		zsetKey := fmt.Sprintf("universe:%s:fleet_events", universeID)
		rdb.ZAdd(r.Context(), zsetKey, redis.Z{
			Score:  float64(arrivalTime.UnixMilli()),
			Member: fmt.Sprintf("%d", fleetID),
		})

		log.Printf("[FLEET DISPATCHED] Fleet #%d mission %s launched to [%d:%d:%d]", fleetID, req.Mission, req.Target.Galaxy, req.Target.System, req.Target.Position)

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   "dispatched",
			"fleet_id": fleetID,
			"arrival":  arrivalTime.Format(time.RFC3339),
			"duration": durationSecs,
			"fuel":     fuelBurn,
		})
	})

	// Fleet Recall Route (Atomic race condition protection for sendfleetback)
	mux.HandleFunc("POST /api/v1/fleets/{id}/recall", func(w http.ResponseWriter, r *http.Request) {
		fleetIDStr := r.PathValue("id")
		w.Header().Set("Content-Type", "application/json")

		fleetID, err := strconv.ParseInt(fleetIDStr, 10, 64)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Invalid fleet ID format"})
			return
		}

		tx, err := db.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		// 1. Lock the fleet row with FOR UPDATE
		var phase string
		var startTime, arrivalTime time.Time
		err = tx.QueryRowContext(r.Context(), `
			SELECT phase, start_time, arrival_time 
			FROM fleets 
			WHERE id = $1 FOR UPDATE
		`, fleetID).Scan(&phase, &startTime, &arrivalTime)

		if err == sql.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Fleet not found"})
			return
		} else if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// 2. Validate that the fleet is still in OUTBOUND phase and has not arrived
		now := time.Now()
		if phase != "OUTBOUND" || now.After(arrivalTime) {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Fleet cannot be recalled: it has already reached target or is already returning",
			})
			return
		}

		// 3. Compute elapsed time and new return arrival time
		elapsed := now.Sub(startTime)
		newReturnArrival := now.Add(elapsed)

		_, err = tx.ExecContext(r.Context(), `
			UPDATE fleets 
			SET phase = 'RETURNING', arrival_time = $1, return_time = $1 
			WHERE id = $2
		`, newReturnArrival, fleetID)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// 4. Update the event score in Redis ZSET
		zsetKey := fmt.Sprintf("universe:%s:fleet_events", universeID)
		rdb.ZAdd(r.Context(), zsetKey, redis.Z{
			Score:  float64(newReturnArrival.UnixMilli()),
			Member: fmt.Sprintf("%d", fleetID),
		})

		log.Printf("[FLEET RECALLED] Fleet #%d reversed safely. Returning in %v", fleetID, elapsed)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   "recalled",
			"fleet_id": fleetID,
			"arrival":  newReturnArrival.Format(time.RFC3339),
			"elapsed":  elapsed.Seconds(),
		})
	})

	// Simple Interactive Visual Dashboard for testing
	mux.HandleFunc("GET /dashboard/{id}", func(w http.ResponseWriter, r *http.Request) {
		planetID := r.PathValue("id")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		var metal, crystal, deuterium float64
		var lastCalc time.Time
		err := db.QueryRowContext(r.Context(), "SELECT * FROM update_celestial_resources($1)", planetID).Scan(
			&metal, &crystal, &deuterium, &lastCalc,
		)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "<h3>Error updating resources. Did you seed the planet record?</h3>")
			return
		}

		var name string
		var g, s, p int
		db.QueryRowContext(r.Context(), "SELECT name, galaxy, system, position FROM celestial_objects WHERE id = $1", planetID).Scan(&name, &g, &s, &p)

		fmt.Fprintf(w, `
			<!DOCTYPE html>
			<html>
			<head>
				<title>Neo-XNova Core Dashboard</title>
				<meta http-equiv="refresh" content="2">
				<style>
					body { font-family: monospace; background: #121214; color: #e1e1e6; padding: 40px; }
					.card { background: #202024; border: 1px solid #323238; padding: 20px; border-radius: 8px; max-width: 500px; }
					.resource { font-size: 20px; color: #04d361; font-weight: bold; margin: 10px 0; }
					.meta { color: #8d8d99; }
				</style>
			</head>
			<body>
				<h2>🚀 Neo-XNova Dev Engine Workbench</h2>
				<div class="card">
					<h3>🪐 %s [%d:%d:%d]</h3>
					<p class="meta">Planet Target ID: %s</p>
					<hr style="border-color: #323238;">
					<div class="resource">🪙 Metal: %.2f</div>
					<div class="resource">💎 Crystal: %.2f</div>
					<div class="resource">⛽ Deuterium: %.2f</div>
					<p class="meta" style="font-size: 11px;">Last Server Dynamic Tick Calculation: %s</p>
				</div>
				<p style="color: #8d8d99; font-size: 12px;">💡 Auto-refreshes every 2 seconds to showcase your delta-time database engine working live!</p>
			</body>
			</html>
		`, name, g, s, p, planetID, metal, crystal, deuterium, lastCalc.Format("15:04:05.000"))
	})

	// 6. Start the Web HTTP App Server
	serverAddr := ":8080"
	server := &http.Server{
		Addr:    serverAddr,
		Handler: mux,
	}

	go func() {
		log.Printf("[INIT] Web API Gateway listening securely on HTTP port %s", serverAddr)
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("[FATAL] Web server crashed on startup: %v", err)
		}
	}()

	// 7. Graceful Shutdown Watcher
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	<-stopChan
	log.Println("[SHUTDOWN] Stopping Neo-XNova Engine gracefully...")

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[ERROR] Web server shutdown forced an exception error: %v", err)
	}

	log.Println("[SHUTDOWN] Engine completely closed down cleanly. Goodbye!")
}

// Simple UUID helper for database mapping
type UUID string
