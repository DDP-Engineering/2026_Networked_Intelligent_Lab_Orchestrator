package modbus

// RevisionRef identifies exactly one saved definition revision.
type RevisionRef struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
}

func (r RevisionRef) valid() bool {
	return identifier.MatchString(r.ID) && r.Revision >= 1 && r.Revision <= 2147483647
}

// PollGroup is inert saved read-plan metadata.
type PollGroup struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Enabled     bool        `json:"enabled"`
	UnitID      int         `json:"unit_id"`
	Function    int         `json:"function"`
	Address     int         `json:"address"`
	Quantity    int         `json:"quantity"`
	IntervalMS  int64       `json:"interval_ms"`
	TimeoutMS   int         `json:"timeout_ms,omitempty"`
	StopOnError bool        `json:"stop_on_error,omitempty"`
	MapRef      RevisionRef `json:"map_ref"`
}

// Layout records view preferences without changing polling or transport state.
type Layout struct {
	Columns       []string `json:"columns"`
	AddressBase   string   `json:"address_base"`
	Series        []string `json:"series"`
	DisplayPaused bool     `json:"display_paused"`
}

// Workspace is a saved, shared view definition, never a live session.
type Workspace struct {
	SchemaVersion int           `json:"schema_version"`
	ID            string        `json:"id"`
	Revision      int64         `json:"revision"`
	OwnerID       string        `json:"owner_id"`
	Name          string        `json:"name"`
	EndpointID    string        `json:"endpoint_id"`
	ProfileRef    RevisionRef   `json:"profile_ref"`
	MapRefs       []RevisionRef `json:"map_refs"`
	PollGroups    []PollGroup   `json:"poll_groups"`
	Layout        Layout        `json:"layout"`
}

// Canonical validates local shape. Bench checks referenced revisions against SQLite.
func (w Workspace) Canonical() (Workspace, error) {
	if w.SchemaVersion != 1 || !identifier.MatchString(w.ID) || !identifier.MatchString(w.OwnerID) || !identifier.MatchString(w.EndpointID) || w.Revision < 1 || w.Revision > 2147483647 || !validName(w.Name) || !w.ProfileRef.valid() || len(w.MapRefs) > 64 || len(w.PollGroups) > 32 {
		return Workspace{}, ErrInvalid
	}
	refs := map[string]int64{}
	for _, r := range w.MapRefs {
		if !r.valid() || refs[r.ID] != 0 {
			return Workspace{}, ErrInvalid
		}
		refs[r.ID] = r.Revision
	}
	ids := map[string]bool{}
	for _, g := range w.PollGroups {
		if !identifier.MatchString(g.ID) || ids[g.ID] || !validName(g.Name) || !g.MapRef.valid() || refs[g.MapRef.ID] != g.MapRef.Revision || g.UnitID < 0 || g.UnitID > 255 || g.Function < 1 || g.Function > 4 || g.Address < 0 || g.Address > 65535 || g.IntervalMS < 1 || g.IntervalMS > 2147483647 || g.TimeoutMS != 0 && (g.TimeoutMS < 100 || g.TimeoutMS > 60000) {
			return Workspace{}, ErrInvalid
		}
		limit := 125
		if g.Function == 1 || g.Function == 2 {
			limit = 2000
		}
		if g.Quantity < 1 || g.Quantity > limit || g.Address+g.Quantity-1 > 65535 {
			return Workspace{}, ErrInvalid
		}
		ids[g.ID] = true
	}
	switch w.Layout.AddressBase {
	case "zero", "one", "reference":
	default:
		return Workspace{}, ErrInvalid
	}
	if len(w.Layout.Columns) > 32 || len(w.Layout.Series) > 128 {
		return Workspace{}, ErrInvalid
	}
	for _, c := range w.Layout.Columns {
		if !validName(c) {
			return Workspace{}, ErrInvalid
		}
	}
	for _, s := range w.Layout.Series {
		if !identifier.MatchString(s) {
			return Workspace{}, ErrInvalid
		}
	}
	w.MapRefs = append([]RevisionRef{}, w.MapRefs...)
	w.PollGroups = append([]PollGroup{}, w.PollGroups...)
	w.Layout.Columns = append([]string{}, w.Layout.Columns...)
	w.Layout.Series = append([]string{}, w.Layout.Series...)
	return w, nil
}
