package lifecycle

import (
	"fmt"
	"os"
	"sync"

	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/env"
	"github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform"
)

const (
	CodeDetectPass = 0
	CodeDetectFail = 100
)

var (
	ErrFailedDetection = errors.New("no buildpacks participating")
	ErrBuildpack       = errors.New("buildpack(s) failed with err")
)

//go:generate mockgen -package testmock -destination testmock/detect_resolver.go github.com/buildpacks/lifecycle DetectResolver
type DetectResolver interface {
	Resolve(done []buildpack.GroupElement, detectRuns *sync.Map) ([]buildpack.GroupElement, []platform.BuildPlanEntry, error)
}

type DetectorFactory struct {
	platformAPI   *api.Version
	apiVerifier   BuildpackAPIVerifier
	configHandler ConfigHandler
	dirStore      DirStore
}

func NewDetectorFactory(
	platformAPI *api.Version,
	apiVerifier BuildpackAPIVerifier,
	configHandler ConfigHandler,
	dirStore DirStore,
) *DetectorFactory {
	return &DetectorFactory{
		platformAPI:   platformAPI,
		apiVerifier:   apiVerifier,
		configHandler: configHandler,
		dirStore:      dirStore,
	}
}

type Detector struct {
	AppDir        string
	DirStore      DirStore
	Executor      buildpack.DetectExecutor
	HasExtensions bool
	Logger        log.Logger
	Order         buildpack.Order
	PlatformDir   string
	Resolver      DetectResolver
	Runs          *sync.Map
}

func (f *DetectorFactory) NewDetector(appDir, orderPath, platformDir string, logger log.Logger) (*Detector, error) {
	detector := &Detector{
		AppDir:      appDir,
		DirStore:    f.dirStore,
		Executor:    &buildpack.DefaultDetectExecutor{},
		Logger:      logger,
		PlatformDir: platformDir,
		Resolver:    &DefaultResolver{Logger: logger},
		Runs:        &sync.Map{},
	}
	if err := f.setOrder(detector, orderPath, logger); err != nil {
		return nil, err
	}
	return detector, nil
}

func (f *DetectorFactory) setOrder(detector *Detector, path string, logger log.Logger) error {
	orderBp, orderExt, err := f.configHandler.ReadOrder(path)
	if err != nil {
		return errors.Wrap(err, "reading order")
	}
	if len(orderExt) > 0 {
		detector.HasExtensions = true
	}
	if err = f.verifyAPIs(orderBp, orderExt, logger); err != nil {
		return err
	}
	detector.Order = PrependExtensions(orderBp, orderExt)
	return nil
}

func (f *DetectorFactory) verifyAPIs(orderBp buildpack.Order, orderExt buildpack.Order, logger log.Logger) error {
	for _, group := range append(orderBp, orderExt...) {
		for _, groupEl := range group.Group {
			module, err := f.dirStore.Lookup(groupEl.Kind(), groupEl.ID, groupEl.Version)
			if err != nil {
				return err
			}
			if err = f.apiVerifier.VerifyBuildpackAPI(groupEl.Kind(), groupEl.String(), module.API(), logger); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *Detector) Detect() (buildpack.Group, platform.BuildPlan, error) {
	return d.DetectOrder(d.Order)
}

func (d *Detector) DetectOrder(order buildpack.Order) (buildpack.Group, platform.BuildPlan, error) {
	detected, planEntries, err := d.detectOrder(order, nil, nil, false, &sync.WaitGroup{})
	if err == ErrBuildpack {
		err = buildpack.NewError(err, buildpack.ErrTypeBuildpack)
	} else if err == ErrFailedDetection {
		err = buildpack.NewError(err, buildpack.ErrTypeFailedDetection)
	}
	for i := range planEntries {
		for j := range planEntries[i].Requires {
			planEntries[i].Requires[j].ConvertVersionToMetadata()
		}
	}
	return buildpack.Group{Group: filter(detected, buildpack.KindBuildpack), GroupExtensions: filter(detected, buildpack.KindExtension)},
		platform.BuildPlan{Entries: planEntries},
		err
}

func filter(group []buildpack.GroupElement, kind string) []buildpack.GroupElement {
	var out []buildpack.GroupElement
	for _, el := range group {
		if el.Kind() == kind {
			out = append(out, el.NoExtension().NoOpt())
		}
	}
	return out
}

func (d *Detector) detectOrder(order buildpack.Order, done, next []buildpack.GroupElement, optional bool, wg *sync.WaitGroup) ([]buildpack.GroupElement, []platform.BuildPlanEntry, error) {
	ngroup := buildpack.Group{Group: next}
	buildpackErr := false
	for _, group := range order {
		// FIXME: double-check slice safety here
		found, plan, err := d.detectGroup(group.Append(ngroup), done, wg)
		if err == ErrBuildpack {
			buildpackErr = true
		}
		if err == ErrFailedDetection || err == ErrBuildpack {
			wg = &sync.WaitGroup{}
			continue
		}
		return found, plan, err
	}
	if optional {
		return d.detectGroup(ngroup, done, wg)
	}

	if buildpackErr {
		return nil, nil, ErrBuildpack
	}
	return nil, nil, ErrFailedDetection
}

func (d *Detector) detectGroup(group buildpack.Group, done []buildpack.GroupElement, wg *sync.WaitGroup) ([]buildpack.GroupElement, []platform.BuildPlanEntry, error) {
	for i, groupEl := range group.Group {
		// Continue if element has already been processed.
		if hasIDForKind(done, groupEl.Kind(), groupEl.ID) {
			continue
		}

		// Resolve order if element is the order for extensions.
		if groupEl.IsExtensionsOrder() {
			return d.detectOrder(groupEl.OrderExtensions, done, group.Group[i+1:], true, wg)
		}

		// Lookup element in store.
		var (
			descriptor buildpack.Descriptor
			err        error
		)
		if groupEl.Kind() == buildpack.KindBuildpack {
			descriptor, err = d.DirStore.LookupBp(groupEl.ID, groupEl.Version)
			if err != nil {
				return nil, nil, err
			}

			// Resolve order if element is a composite buildpack.
			if order := descriptor.(*buildpack.BpDescriptor).Order; len(order) > 0 {
				// FIXME: double-check slice safety here
				// FIXME: cyclical references lead to infinite recursion
				return d.detectOrder(order, done, group.Group[i+1:], groupEl.Optional, wg)
			}
		} else {
			descriptor, err = d.DirStore.LookupExt(groupEl.ID, groupEl.Version)
			if err != nil {
				return nil, nil, err
			}
		}

		// Mark element as done.
		done = append(done, groupEl.WithAPI(descriptor.API()).WithHomepage(descriptor.Homepage()))

		// Run detect if element is a component buildpack or an extension.
		wg.Add(1)
		key := keyFor(groupEl)
		go func(key string, descriptor buildpack.Descriptor) {
			if _, ok := d.Runs.Load(key); !ok {
				inputs := buildpack.DetectInputs{
					AppDir:      d.AppDir,
					PlatformDir: d.PlatformDir,
					Env:         env.NewBuildEnv(os.Environ()),
				}
				d.Runs.Store(key, d.Executor.Detect(descriptor, inputs, d.Logger))
			}
			wg.Done()
		}(key, descriptor)
	}

	wg.Wait()

	return d.Resolver.Resolve(done, d.Runs)
}

func hasIDForKind(els []buildpack.GroupElement, kind string, id string) bool {
	for _, el := range els {
		if el.Kind() == kind && el.ID == id {
			return true
		}
	}
	return false
}

func keyFor(groupEl buildpack.GroupElement) string {
	return fmt.Sprintf("%s %s", groupEl.Kind(), groupEl.String())
}

type DefaultResolver struct {
	Logger log.Logger
}

// Resolve aggregates the detect output for a group of buildpacks and tries to resolve a build plan for the group.
// If any required buildpack in the group failed detection or a build plan cannot be resolved, it returns an error.
func (r *DefaultResolver) Resolve(done []buildpack.GroupElement, detectRuns *sync.Map) ([]buildpack.GroupElement, []platform.BuildPlanEntry, error) {
	var groupRuns []buildpack.DetectOutputs
	for _, el := range done {
		key := keyFor(el) // FIXME: ensure the Detector and Resolver always use the same key
		t, ok := detectRuns.Load(key)
		if !ok {
			return nil, nil, errors.Errorf("missing detection of '%s'", key)
		}
		run := t.(buildpack.DetectOutputs)
		outputLogf := r.Logger.Debugf

		switch run.Code {
		case CodeDetectPass, CodeDetectFail:
		default:
			outputLogf = r.Logger.Infof
		}

		if len(run.Output) > 0 {
			outputLogf("======== Output: %s ========", el)
			outputLogf(string(run.Output))
		}
		if run.Err != nil {
			outputLogf("======== Error: %s ========", el)
			outputLogf(run.Err.Error())
		}
		groupRuns = append(groupRuns, run)
	}

	r.Logger.Debugf("======== Results ========")

	results := detectResults{}
	detected := true
	anyBuildpacksDetected := false
	buildpackErr := false
	for i, el := range done {
		run := groupRuns[i]
		switch run.Code {
		case CodeDetectPass:
			r.Logger.Debugf("pass: %s", el)
			results = append(results, detectResult{el, run})
			if !el.Extension {
				anyBuildpacksDetected = true
			}
		case CodeDetectFail:
			if el.Optional {
				r.Logger.Debugf("skip: %s", el)
			} else {
				r.Logger.Debugf("fail: %s", el)
			}
			detected = detected && el.Optional
		case -1:
			r.Logger.Infof("err:  %s", el)
			buildpackErr = true
			detected = detected && el.Optional
		default:
			r.Logger.Infof("err:  %s (%d)", el, run.Code)
			buildpackErr = true
			detected = detected && el.Optional
		}
	}
	if !detected {
		if buildpackErr {
			return nil, nil, ErrBuildpack
		}
		return nil, nil, ErrFailedDetection
	} else if !anyBuildpacksDetected {
		r.Logger.Debugf("fail: no viable buildpacks in group")
		return nil, nil, ErrFailedDetection
	}

	i := 0
	deps, trial, err := results.runTrials(func(trial detectTrial) (depMap, detectTrial, error) {
		i++
		return r.runTrial(i, trial)
	})
	if err != nil {
		return nil, nil, err
	}

	if len(done) != len(trial) {
		r.Logger.Infof("%d of %d buildpacks participating", len(trial), len(done))
	}

	maxLength := 0
	for _, t := range trial {
		l := len(t.ID)
		if l > maxLength {
			maxLength = l
		}
	}

	f := fmt.Sprintf("%%-%ds %%s", maxLength)

	for _, t := range trial {
		r.Logger.Infof(f, t.ID, t.Version)
	}

	var found []buildpack.GroupElement
	for _, r := range trial {
		found = append(found, r.GroupElement.NoOpt())
	}
	var plan []platform.BuildPlanEntry
	for _, dep := range deps {
		plan = append(plan, dep.BuildPlanEntry.NoOpt())
	}
	return found, plan, nil
}

func (r *DefaultResolver) runTrial(i int, trial detectTrial) (depMap, detectTrial, error) {
	r.Logger.Debugf("Resolving plan... (try #%d)", i)

	var deps depMap
	retry := true
	for retry {
		retry = false
		deps = newDepMap(trial)

		if err := deps.eachUnmetRequire(func(name string, el buildpack.GroupElement) error {
			retry = true
			if !el.Optional {
				r.Logger.Debugf("fail: %s requires %s", el, name)
				return ErrFailedDetection
			}
			r.Logger.Debugf("skip: %s requires %s", el, name)
			trial = trial.remove(el)
			return nil
		}); err != nil {
			return nil, nil, err
		}

		if err := deps.eachUnmetProvide(func(name string, el buildpack.GroupElement) error {
			retry = true
			if !el.Optional {
				r.Logger.Debugf("fail: %s provides unused %s", el, name)
				return ErrFailedDetection
			}
			r.Logger.Debugf("skip: %s provides unused %s", el, name)
			trial = trial.remove(el)
			return nil
		}); err != nil {
			return nil, nil, err
		}
	}

	if len(trial) == 0 {
		r.Logger.Debugf("fail: no viable buildpacks in group")
		return nil, nil, ErrFailedDetection
	}
	return deps, trial, nil
}

type detectResult struct {
	buildpack.GroupElement
	buildpack.DetectOutputs
}

func (r *detectResult) options() []detectOption {
	var out []detectOption
	for i, sections := range append([]buildpack.PlanSections{r.PlanSections}, r.Or...) {
		el := r.GroupElement
		el.Optional = el.Optional && i == len(r.Or)
		out = append(out, detectOption{el, sections})
	}
	return out
}

type detectResults []detectResult
type trialFunc func(detectTrial) (depMap, detectTrial, error)

func (rs detectResults) runTrials(f trialFunc) (depMap, detectTrial, error) {
	return rs.runTrialsFrom(nil, f)
}

func (rs detectResults) runTrialsFrom(prefix detectTrial, f trialFunc) (depMap, detectTrial, error) {
	if len(rs) == 0 {
		deps, trial, err := f(prefix)
		return deps, trial, err
	}

	var lastErr error
	for _, option := range rs[0].options() {
		deps, trial, err := rs[1:].runTrialsFrom(append(prefix, option), f)
		if err == nil {
			return deps, trial, nil
		}
		lastErr = err
	}
	return nil, nil, lastErr
}

type detectOption struct {
	buildpack.GroupElement
	buildpack.PlanSections
}

type detectTrial []detectOption

func (ts detectTrial) remove(el buildpack.GroupElement) detectTrial {
	var out detectTrial
	for _, t := range ts {
		if !t.GroupElement.Equals(el) {
			out = append(out, t)
		}
	}
	return out
}

type depEntry struct {
	platform.BuildPlanEntry
	earlyRequires []buildpack.GroupElement
	extraProvides []buildpack.GroupElement
}

type depMap map[string]depEntry

func newDepMap(trial detectTrial) depMap {
	m := depMap{}
	for _, option := range trial {
		for _, p := range option.Provides {
			m.provide(option.GroupElement, p)
		}
		for _, r := range option.Requires {
			m.require(option.GroupElement, r)
		}
	}
	return m
}

func (m depMap) provide(el buildpack.GroupElement, provide buildpack.Provide) {
	entry := m[provide.Name]
	entry.extraProvides = append(entry.extraProvides, el)
	m[provide.Name] = entry
}

func (m depMap) require(el buildpack.GroupElement, require buildpack.Require) {
	entry := m[require.Name]
	entry.Providers = append(entry.Providers, entry.extraProvides...)
	entry.extraProvides = nil

	if len(entry.Providers) == 0 {
		entry.earlyRequires = append(entry.earlyRequires, el)
	} else {
		entry.Requires = append(entry.Requires, require)
	}
	m[require.Name] = entry
}

func (m depMap) eachUnmetProvide(f func(name string, el buildpack.GroupElement) error) error {
	for name, entry := range m {
		if len(entry.extraProvides) != 0 {
			for _, bp := range entry.extraProvides {
				if err := f(name, bp); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (m depMap) eachUnmetRequire(f func(name string, el buildpack.GroupElement) error) error {
	for name, entry := range m {
		if len(entry.earlyRequires) != 0 {
			for _, el := range entry.earlyRequires {
				if err := f(name, el); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func PrependExtensions(orderBp buildpack.Order, orderExt buildpack.Order) buildpack.Order {
	if len(orderExt) == 0 {
		return orderBp
	}

	// fill in values for extensions order
	for i, group := range orderExt {
		for j := range group.Group {
			group.Group[j].Extension = true
			group.Group[j].Optional = true
		}
		orderExt[i] = group
	}

	var newOrder buildpack.Order
	extGroupEl := buildpack.GroupElement{OrderExtensions: orderExt}
	for _, group := range orderBp {
		newOrder = append(newOrder, buildpack.Group{
			Group: append([]buildpack.GroupElement{extGroupEl}, group.Group...),
		})
	}
	return newOrder
}
