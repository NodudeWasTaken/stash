package heresphere

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/stashapp/stash/internal/api/urlbuilders"
	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/scene"
)

type HeresphereCustomTag string

const (
	HeresphereCustomTagInteractive HeresphereCustomTag = "Interactive"

	HeresphereCustomTagPlayCount HeresphereCustomTag = "PlayCount"
	HeresphereCustomTagWatched   HeresphereCustomTag = "Watched"

	HeresphereCustomTagOrganized HeresphereCustomTag = "Organized"

	HeresphereCustomTagOCounter HeresphereCustomTag = "OCounter"
	HeresphereCustomTagOrgasmed HeresphereCustomTag = "Orgasmed"

	HeresphereCustomTagRated HeresphereCustomTag = "Rated"
)

type routes struct {
	repository
	SceneFinder       sceneFinder
	SceneService      manager.SceneService
	SceneMarkerFinder sceneMarkerFinder
	FileFinder        fileFinder
	TagFinder         tagFinder
	FilterFinder      savedfilterFinder
	PerformerFinder   performerFinder
	GalleryFinder     galleryFinder
	MovieFinder       movieFinder
	StudioFinder      studioFinder
	OCountFinder      ocountFinder
	ViewFinder        viewFinder
	HookExecutor      hookExecutor
}

func GetRoutes(repo models.Repository) chi.Router {
	return routes{
		repository:        repository{TxnManager: repo.TxnManager},
		SceneFinder:       repo.Scene,
		SceneService:      manager.GetInstance().SceneService,
		SceneMarkerFinder: repo.SceneMarker,
		FileFinder:        repo.File,
		TagFinder:         repo.Tag,
		FilterFinder:      repo.SavedFilter,
		PerformerFinder:   repo.Performer,
		GalleryFinder:     repo.Gallery,
		MovieFinder:       repo.Movie,
		StudioFinder:      repo.Studio,
		OCountFinder:      repo.Scene,
		ViewFinder:        repo.Scene,
		HookExecutor:      manager.GetInstance().PluginCache,
	}.Routes()
}

/*
 * This function provides the possible routes for this api.
 */
func (rs routes) Routes() chi.Router {
	r := chi.NewRouter()

	r.Route("/", func(r chi.Router) {
		r.Use(rs.heresphereCtx)

		r.Post("/", rs.heresphereIndex)
		r.Get("/", rs.heresphereIndex)
		r.Head("/", rs.heresphereIndex)

		r.Post("/auth", rs.heresphereLoginToken)
		r.Route("/{sceneId}", func(r chi.Router) {
			r.Use(rs.heresphereSceneCtx)

			r.Post("/", rs.heresphereVideoData)
			r.Get("/", rs.heresphereVideoData)

			r.Post("/event", rs.heresphereVideoEvent)
			r.Get("/file.hsp", rs.heresphereHSP)
		})
	})

	return r
}

var (
	idMap = make(map[string]string)
)

/*
 * This is a video playback event
 * Intended for server-sided script playback.
 * But since we dont need that, we just use it for timestamps.
 */
func (rs routes) heresphereVideoEvent(w http.ResponseWriter, r *http.Request) {
	// Get the scene from the request context
	scn := r.Context().Value(sceneKey).(*models.Scene)

	// Decode the JSON request body into the HeresphereVideoEvent struct
	var event HeresphereVideoEvent
	err := json.NewDecoder(r.Body).Decode(&event)
	if err != nil {
		// Handle JSON decoding error
		logger.Errorf("Heresphere HeresphereVideoEvent decode error: %s\n", err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Convert time from milliseconds to seconds
	newTime := event.Time / 1000
	newDuration := 0.0

	// Calculate new duration if necessary
	// (if HeresphereEventPlay then its most likely a "skip" event)
	if newTime > scn.ResumeTime && event.Event != HeresphereEventPlay {
		newDuration += (newTime - scn.ResumeTime)
	}

	// Check if the event ID is different from the previous event for the same client
	previousID := idMap[r.RemoteAddr]
	if previousID != event.Id {
		// Update play count and store the new event ID if needed
		if b, err := rs.updatePlayCount(r.Context(), scn, event); err != nil {
			// Handle updatePlayCount error
			logger.Errorf("Heresphere HeresphereVideoEvent updatePlayCount error: %s\n", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} else if b {
			idMap[r.RemoteAddr] = event.Id
		}
	}

	// Update the scene activity with the new time and duration
	if err := rs.withTxn(r.Context(), func(ctx context.Context) error {
		_, err := rs.SceneFinder.SaveActivity(ctx, scn.ID, &newTime, &newDuration)
		return err
	}); err != nil {
		// Handle SaveActivity error
		logger.Errorf("Heresphere HeresphereVideoEvent SaveActivity error: %s\n", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Respond with a successful HTTP status code
	w.WriteHeader(http.StatusOK)
}

/*
 * This endpoint is for letting the user update scene data
 */
func (rs routes) heresphereVideoDataUpdate(w http.ResponseWriter, r *http.Request) error {
	scn := r.Context().Value(sceneKey).(*models.Scene)
	user := r.Context().Value(authKey).(HeresphereAuthReq)
	c := config.GetInstance()
	shouldUpdate := false

	ret := &scene.UpdateSet{
		ID:      scn.ID,
		Partial: models.NewScenePartial(),
	}

	var b bool
	var err error
	if user.Rating != nil && c.GetHSPWriteRatings() {
		if b, err = rs.updateRating(user, ret); err != nil {
			return err
		}
		shouldUpdate = b || shouldUpdate
	}

	if user.DeleteFile != nil && *user.DeleteFile && c.GetHSPWriteDeletes() {
		if _, err = rs.handleDeleteScene(r.Context(), scn); err != nil {
			return err
		}
		return fmt.Errorf("file was deleted")
	}

	if user.IsFavorite != nil && c.GetHSPWriteFavorites() {
		if b, err = rs.handleFavoriteTag(r.Context(), scn, &user, ret); err != nil {
			return err
		}
		shouldUpdate = b || shouldUpdate
	}

	if user.Tags != nil && c.GetHSPWriteTags() {
		if b, err = rs.handleTags(r.Context(), scn, &user, ret); err != nil {
			return err
		}
		shouldUpdate = b || shouldUpdate
	}
	if user.HspBase64 != nil && c.GetHSPWriteHsp() {
		decodedBytes, err := base64.StdEncoding.DecodeString(*user.HspBase64)
		if err != nil {
			return err
		}
		filename, _ := getHspFile(scn.Files.Primary())

		err = os.WriteFile(filename, decodedBytes, 0644)
		if err != nil {
			return err
		}
	}

	if shouldUpdate {
		if err := rs.withTxn(r.Context(), func(ctx context.Context) error {
			_, err := ret.Update(ctx, rs.SceneFinder)
			return err
		}); err != nil {
			return err
		}

		return nil
	}
	return nil
}

/*
 * This endpoint provides the main libraries that are available to browse.
 */
func (rs routes) heresphereIndex(w http.ResponseWriter, r *http.Request) {
	// Banner
	banner := HeresphereBanner{
		Image: fmt.Sprintf("%s%s", manager.GetBaseURL(r), "/apple-touch-icon.png"),
		Link:  fmt.Sprintf("%s%s", manager.GetBaseURL(r), "/"),
	}

	// Index
	libraryObj := HeresphereIndex{
		Access:  HeresphereMember,
		Banner:  banner,
		Library: []HeresphereIndexEntry{},
	}

	// Add filters
	parsedFilters, err := rs.getAllFilters(r.Context())
	if err == nil {
		var keys []string
		for key := range parsedFilters {
			keys = append(keys, key)
		}

		sort.Strings(keys)

		for _, key := range keys {
			value := parsedFilters[key]
			sceneUrls := make([]string, len(value))

			for idx, sceneID := range value {
				sceneUrls[idx] = addApiKey(fmt.Sprintf("%s/heresphere/%d", manager.GetBaseURL(r), sceneID))
			}

			libraryObj.Library = append(libraryObj.Library, HeresphereIndexEntry{
				Name: key,
				List: sceneUrls,
			})
		}
	} else {
		logger.Warnf("Heresphere HeresphereIndex getAllFilters error: %s\n", err.Error())
	}

	// Set response headers and encode JSON
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(libraryObj); err != nil {
		logger.Errorf("Heresphere HeresphereIndex encode error: %s\n", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (rs routes) heresphereHSP(w http.ResponseWriter, r *http.Request) {
	// Fetch scene
	scene := r.Context().Value(sceneKey).(*models.Scene)

	/*version, err := strconv.Atoi(chi.URLParam(r, "version"))
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}*/

	primaryFile := scene.Files.Primary()
	if filename, err := getHspFile(primaryFile); !os.IsNotExist(err) {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filepath.Base(filename)))
		http.ServeFile(w, r, filename)
		return
	}

	w.WriteHeader(400)
}

/*
 * This endpoint provides a single scenes full information.
 */
func (rs routes) heresphereVideoData(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(authKey).(HeresphereAuthReq)
	c := config.GetInstance()

	// Update request
	if err := rs.heresphereVideoDataUpdate(w, r); err != nil {
		logger.Errorf("Heresphere HeresphereVideoData HeresphereVideoDataUpdate error: %s\n", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Fetch scene
	scene := r.Context().Value(sceneKey).(*models.Scene)

	// Load relationships
	processedScene := HeresphereVideoEntry{}
	if err := rs.withReadTxn(r.Context(), func(ctx context.Context) error {
		return scene.LoadRelationships(ctx, rs.SceneFinder)
	}); err != nil {
		logger.Errorf("Heresphere HeresphereVideoData LoadRelationships error: %s\n", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Create scene
	processedScene = HeresphereVideoEntry{
		Access:         HeresphereMember,
		Title:          scene.GetTitle(),
		Description:    scene.Details,
		ThumbnailImage: addApiKey(urlbuilders.NewSceneURLBuilder(manager.GetBaseURL(r), scene).GetScreenshotURL()),
		ThumbnailVideo: addApiKey(urlbuilders.NewSceneURLBuilder(manager.GetBaseURL(r), scene).GetStreamPreviewURL()),
		DateAdded:      scene.CreatedAt.Format("2006-01-02"),
		Duration:       0.0,
		Rating:         0,
		Favorites:      0,
		Comments:       0,
		IsFavorite:     rs.getVideoFavorite(r, scene),
		Projection:     HeresphereProjectionPerspective,
		Stereo:         HeresphereStereoMono,
		IsEyeSwapped:   false,
		Fov:            180.0,
		Lens:           HeresphereLensLinear,
		CameraIPD:      6.5,
		EventServer: addApiKey(fmt.Sprintf("%s/heresphere/%d/event",
			manager.GetBaseURL(r),
			scene.ID,
		)),
		Scripts:       rs.getVideoScripts(r, scene),
		Subtitles:     rs.getVideoSubtitles(r, scene),
		Tags:          rs.getVideoTags(r.Context(), scene),
		Media:         []HeresphereVideoMedia{},
		WriteFavorite: c.GetHSPWriteFavorites(),
		WriteRating:   c.GetHSPWriteRatings(),
		WriteTags:     c.GetHSPWriteTags(),
		WriteHSP:      c.GetHSPWriteHsp(),
	}

	// Find projection options
	FindProjectionTags(scene, &processedScene)

	// Additional info
	if user.NeedsMediaSource != nil && *user.NeedsMediaSource {
		processedScene.Media = rs.getVideoMedia(r, scene)
	}
	if scene.Date != nil {
		processedScene.DateReleased = scene.Date.Format("2006-01-02")
	}
	if scene.Rating != nil {
		fiveScale := models.Rating100To5F(*scene.Rating)
		processedScene.Rating = fiveScale
	}
	if processedScene.IsFavorite {
		processedScene.Favorites++
	}
	if scene.Files.PrimaryLoaded() {
		file_ids := scene.Files.Primary()
		if file_ids != nil {
			if val := manager.HandleFloat64(file_ids.Duration * 1000.0); val != nil {
				processedScene.Duration = *val
			}
		}

		if _, err := getHspFile(file_ids); !os.IsNotExist(err) {
			processedScene.HspArray = []HeresphereHSPEntry{
				{
					Url: addApiKey(fmt.Sprintf("%s/heresphere/%d/file.hsp",
						manager.GetBaseURL(r),
						scene.ID,
					)),
					//Version: 8,
				},
			}
		}
	}

	// Create a JSON encoder for the response writer
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(processedScene); err != nil {
		logger.Errorf("Heresphere HeresphereVideoData encode error: %s\n", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

/*
 * This endpoint function allows the user to login and receive a token if successful.
 */
func (rs routes) heresphereLoginToken(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value(authKey).(HeresphereAuthReq)

	// Try login
	if basicLogin(user.Username, user.Password) {
		writeNotAuthorized(w, r, "Invalid credentials")
		return
	}

	// Fetch key
	key := config.GetInstance().GetAPIKey()
	if len(key) == 0 {
		writeNotAuthorized(w, r, "Missing auth key!")
		return
	}

	// Generate auth response
	auth := &HeresphereAuthResp{
		AuthToken: key,
		Access:    HeresphereMember,
	}

	// Create a JSON encoder for the response writer
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(auth); err != nil {
		logger.Errorf("Heresphere HeresphereLoginToken encode error: %s\n", err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

/*
 * This context function finds the applicable scene from the request and stores it.
 */
func (rs routes) heresphereSceneCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get sceneId
		sceneID, err := strconv.Atoi(chi.URLParam(r, "sceneId"))
		if err != nil {
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		// Resolve scene
		var scene *models.Scene
		_ = rs.withReadTxn(r.Context(), func(ctx context.Context) error {
			qb := rs.SceneFinder
			scene, _ = qb.Find(ctx, sceneID)

			if scene != nil {
				// A valid scene should have a attached video
				if err := scene.LoadPrimaryFile(ctx, rs.FileFinder); err != nil {
					if !errors.Is(err, context.Canceled) {
						logger.Errorf("error loading primary file for scene %d: %v", sceneID, err)
					}
					// set scene to nil so that it doesn't try to use the primary file
					scene = nil
				}
			}

			return nil
		})
		if scene == nil {
			http.Error(w, http.StatusText(404), 404)
			return
		}

		ctx := context.WithValue(r.Context(), sceneKey, scene)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

/*
 * This context function finds if the authentication is correct, otherwise rejects the request.
 */
func (rs routes) heresphereCtx(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add JSON Header (using Add uses camel case and makes it invalid because "Json")
		w.Header()["HereSphere-JSON-Version"] = []string{strconv.Itoa(HeresphereJsonVersion)}

		// Only if enabled
		if !config.GetInstance().GetHSPDefaultEnabled() {
			writeNotAuthorized(w, r, "HereSphere API not enabled!")
			return
		}

		// Read HTTP Body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "can't read body", http.StatusBadRequest)
			return
		}

		// Make the Body re-readable (afaik only /event uses this)
		r.Body = io.NopCloser(bytes.NewBuffer(body))

		// Auth enabled and not has valid credentials (TLDR: needs to be blocked)
		isAuth := config.GetInstance().HasCredentials() && !HeresphereHasValidToken(r)

		// Default request
		user := HeresphereAuthReq{}

		// Attempt decode, and if err and invalid auth, fail
		if err := json.Unmarshal(body, &user); err != nil && isAuth {
			writeNotAuthorized(w, r, "Not logged in!")
			return
		}

		// If empty, fill as true
		if user.NeedsMediaSource == nil {
			needsMedia := true
			user.NeedsMediaSource = &needsMedia
		}

		// If invalid creds, only allow auth endpoint
		if isAuth && !strings.HasPrefix(r.URL.Path, "/heresphere/auth") {
			writeNotAuthorized(w, r, "Unauthorized!")
			return
		}

		ctx := context.WithValue(r.Context(), authKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
