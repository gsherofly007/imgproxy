package options

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/imgproxy/imgproxy/v2/config"
	"github.com/imgproxy/imgproxy/v2/ierrors"
	"github.com/imgproxy/imgproxy/v2/imagetype"
	"github.com/imgproxy/imgproxy/v2/structdiff"
	"github.com/imgproxy/imgproxy/v2/vips"
)

const maxClientHintDPR = 8

var errExpiredURL = errors.New("Expired URL")

type GravityOptions struct {
	Type GravityType
	X, Y float64
}

type ExtendOptions struct {
	Enabled bool
	Gravity GravityOptions
}

type CropOptions struct {
	Width   float64
	Height  float64
	Gravity GravityOptions
}

type PaddingOptions struct {
	Enabled bool
	Top     int
	Right   int
	Bottom  int
	Left    int
}

type TrimOptions struct {
	Enabled   bool
	Threshold float64
	Smart     bool
	Color     vips.Color
	EqualHor  bool
	EqualVer  bool
}

type WatermarkOptions struct {
	Enabled   bool
	Opacity   float64
	Replicate bool
	Gravity   GravityOptions
	Scale     float64
}

type ProcessingOptions struct {
	ResizingType      ResizeType
	Width             int
	Height            int
	MinWidth          int
	MinHeight         int
	Dpr               float64
	Gravity           GravityOptions
	Enlarge           bool
	Extend            ExtendOptions
	Crop              CropOptions
	Padding           PaddingOptions
	Trim              TrimOptions
	Rotate            int
	Format            imagetype.Type
	Quality           int
	MaxBytes          int
	Flatten           bool
	Background        vips.Color
	Blur              float32
	Sharpen           float32
	StripMetadata     bool
	StripColorProfile bool
	AutoRotate        bool

	SkipProcessingFormats []imagetype.Type

	CacheBuster string

	Watermark WatermarkOptions

	PreferWebP  bool
	EnforceWebP bool
	PreferAvif  bool
	EnforceAvif bool

	Filename string

	UsedPresets []string
}

var (
	_newProcessingOptions    ProcessingOptions
	newProcessingOptionsOnce sync.Once
)

func NewProcessingOptions() *ProcessingOptions {
	newProcessingOptionsOnce.Do(func() {
		_newProcessingOptions = ProcessingOptions{
			ResizingType:      ResizeFit,
			Width:             0,
			Height:            0,
			Gravity:           GravityOptions{Type: GravityCenter},
			Enlarge:           false,
			Extend:            ExtendOptions{Enabled: false, Gravity: GravityOptions{Type: GravityCenter}},
			Padding:           PaddingOptions{Enabled: false},
			Trim:              TrimOptions{Enabled: false, Threshold: 10, Smart: true},
			Rotate:            0,
			Quality:           0,
			MaxBytes:          0,
			Format:            imagetype.Unknown,
			Background:        vips.Color{R: 255, G: 255, B: 255},
			Blur:              0,
			Sharpen:           0,
			Dpr:               1,
			Watermark:         WatermarkOptions{Opacity: 1, Replicate: false, Gravity: GravityOptions{Type: GravityCenter}},
			StripMetadata:     config.StripMetadata,
			StripColorProfile: config.StripColorProfile,
			AutoRotate:        config.AutoRotate,
		}
	})

	po := _newProcessingOptions
	po.SkipProcessingFormats = append([]imagetype.Type(nil), config.SkipProcessingFormats...)
	po.UsedPresets = make([]string, 0, len(config.Presets))

	return &po
}

func (po *ProcessingOptions) GetQuality() int {
	q := po.Quality

	if q == 0 {
		q = config.FormatQuality[po.Format]
	}

	if q == 0 {
		q = config.Quality
	}

	return q
}

func (po *ProcessingOptions) isPresetUsed(name string) bool {
	for _, usedName := range po.UsedPresets {
		if usedName == name {
			return true
		}
	}
	return false
}

func (po *ProcessingOptions) Diff() structdiff.Entries {
	return structdiff.Diff(NewProcessingOptions(), po)
}

func (po *ProcessingOptions) String() string {
	return po.Diff().String()
}

func (po *ProcessingOptions) MarshalJSON() ([]byte, error) {
	return po.Diff().MarshalJSON()
}

func parseDimension(d *int, name, arg string) error {
	if v, err := strconv.Atoi(arg); err == nil && v >= 0 {
		*d = v
	} else {
		return fmt.Errorf("Invalid %s: %s", name, arg)
	}

	return nil
}

func parseBoolOption(str string) bool {
	b, err := strconv.ParseBool(str)

	if err != nil {
		log.Warningf("`%s` is not a valid boolean value. Treated as false", str)
	}

	return b
}

func isGravityOffcetValid(gravity GravityType, offset float64) bool {
	if gravity == GravityCenter {
		return true
	}

	return offset >= 0 && (gravity != GravityFocusPoint || offset <= 1)
}

func parseGravity(g *GravityOptions, args []string) error {
	nArgs := len(args)

	if nArgs > 3 {
		return fmt.Errorf("Invalid gravity arguments: %v", args)
	}

	if t, ok := gravityTypes[args[0]]; ok {
		g.Type = t
	} else {
		return fmt.Errorf("Invalid gravity: %s", args[0])
	}

	if g.Type == GravitySmart && nArgs > 1 {
		return fmt.Errorf("Invalid gravity arguments: %v", args)
	} else if g.Type == GravityFocusPoint && nArgs != 3 {
		return fmt.Errorf("Invalid gravity arguments: %v", args)
	}

	if nArgs > 1 {
		if x, err := strconv.ParseFloat(args[1], 64); err == nil && isGravityOffcetValid(g.Type, x) {
			g.X = x
		} else {
			return fmt.Errorf("Invalid gravity X: %s", args[1])
		}
	}

	if nArgs > 2 {
		if y, err := strconv.ParseFloat(args[2], 64); err == nil && isGravityOffcetValid(g.Type, y) {
			g.Y = y
		} else {
			return fmt.Errorf("Invalid gravity Y: %s", args[2])
		}
	}

	return nil
}

func applyWidthOption(po *ProcessingOptions, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("Invalid width arguments: %v", args)
	}

	return parseDimension(&po.Width, "width", args[0])
}

func applyHeightOption(po *ProcessingOptions, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("Invalid height arguments: %v", args)
	}

	return parseDimension(&po.Height, "height", args[0])
}

func applyMinWidthOption(po *ProcessingOptions, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("Invalid min width arguments: %v", args)
	}

	return parseDimension(&po.MinWidth, "min width", args[0])
}

func applyMinHeightOption(po *ProcessingOptions, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("Invalid min height arguments: %v", args)
	}

	return parseDimension(&po.MinHeight, " min height", args[0])
}

func applyEnlargeOption(po *ProcessingOptions, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("Invalid enlarge arguments: %v", args)
	}

	po.Enlarge = parseBoolOption(args[0])

	return nil
}

func applyExtendOption(po *ProcessingOptions, args []string) error {
	if len(args) > 4 {
		return fmt.Errorf("Invalid extend arguments: %v", args)
	}

	po.Extend.Enabled = parseBoolOption(args[0])

	if len(args) > 1 {
		if err := parseGravity(&po.Extend.Gravity, args[1:]); err != nil {
			return err
		}

		if po.Extend.Gravity.Type == GravitySmart {
			return errors.New("extend doesn't support smart gravity")
		}
	}

	return nil
}

func applySizeOption(po *ProcessingOptions, args []string) (err error) {
	if len(args) > 7 {
		return fmt.Errorf("Invalid size arguments: %v", args)
	}

	if len(args) >= 1 && len(args[0]) > 0 {
		if err = applyWidthOption(po, args[0:1]); err != nil {
			return
		}
	}

	if len(args) >= 2 && len(args[1]) > 0 {
		if err = applyHeightOption(po, args[1:2]); err != nil {
			return
		}
	}

	if len(args) >= 3 && len(args[2]) > 0 {
		if err = applyEnlargeOption(po, args[2:3]); err != nil {
			return
		}
	}

	if len(args) >= 4 && len(args[3]) > 0 {
		if err = applyExtendOption(po, args[3:]); err != nil {
			return
		}
	}

	return nil
}

func applyResizingTypeOption(po *ProcessingOptions, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("Invalid resizing type arguments: %v", args)
	}

	if r, ok := resizeTypes[args[0]]; ok {
		po.ResizingType = r
	} else {
		return fmt.Errorf("Invalid resize type: %s", args[0])
	}

	return nil
}

func applyResizeOption(po *ProcessingOptions, args []string) error {
	if len(args) > 8 {
		return fmt.Errorf("Invalid resize arguments: %v", args)
	}

	if len(args[0]) > 0 {
		if err := applyResizingTypeOption(po, args[0:1]); err != nil {
			return err
		}
	}

	if len(args) > 1 {
		if err := applySizeOption(po, args[1:]); err != nil {
			return err
		}
	}

	return nil
}

func applyDprOption(po *ProcessingOptions, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("Invalid dpr arguments: %v", args)
	}

	if d, err := strconv.ParseFloat(args[0], 64); err == nil && d > 0 {
		po.Dpr = d
	} else {
		return fmt.Errorf("Invalid dpr: %s", args[0])
	}

	return nil
}

func applyGravityOption(po *ProcessingOptions, args []string) error {
	return parseGravity(&po.Gravity, args)
}

func applyCropOption(po *ProcessingOptions, args []string) error {
	if len(args) > 5 {
		return fmt.Errorf("Invalid crop arguments: %v", args)
	}

	if w, err := strconv.ParseFloat(args[0], 64); err == nil && w >= 0 {
		po.Crop.Width = w
	} else {
		return fmt.Errorf("Invalid crop width: %s", args[0])
	}

	if len(args) > 1 {
		if h, err := strconv.ParseFloat(args[1], 64); err == nil && h >= 0 {
			po.Crop.Height = h
		} else {
			return fmt.Errorf("Invalid crop height: %s", args[1])
		}
	}

	if len(args) > 2 {
		return parseGravity(&po.Crop.Gravity, args[2:])
	}

	return nil
}

func applyPaddingOption(po *ProcessingOptions, args []string) error {
	nArgs := len(args)

	if nArgs < 1 || nArgs > 4 {
		return fmt.Errorf("Invalid padding arguments: %v", args)
	}

	po.Padding.Enabled = true

	if nArgs > 0 && len(args[0]) > 0 {
		if err := parseDimension(&po.Padding.Top, "padding top (+all)", args[0]); err != nil {
			return err
		}
		po.Padding.Right = po.Padding.Top
		po.Padding.Bottom = po.Padding.Top
		po.Padding.Left = po.Padding.Top
	}

	if nArgs > 1 && len(args[1]) > 0 {
		if err := parseDimension(&po.Padding.Right, "padding right (+left)", args[1]); err != nil {
			return err
		}
		po.Padding.Left = po.Padding.Right
	}

	if nArgs > 2 && len(args[2]) > 0 {
		if err := parseDimension(&po.Padding.Bottom, "padding bottom", args[2]); err != nil {
			return err
		}
	}

	if nArgs > 3 && len(args[3]) > 0 {
		if err := parseDimension(&po.Padding.Left, "padding left", args[3]); err != nil {
			return err
		}
	}

	if po.Padding.Top == 0 && po.Padding.Right == 0 && po.Padding.Bottom == 0 && po.Padding.Left == 0 {
		po.Padding.Enabled = false
	}

	return nil
}

func applyTrimOption(po *ProcessingOptions, args []string) error {
	nArgs := len(args)

	if nArgs > 4 {
		return fmt.Errorf("Invalid trim arguments: %v", args)
	}

	if t, err := strconv.ParseFloat(args[0], 64); err == nil && t >= 0 {
		po.Trim.Enabled = true
		po.Trim.Threshold = t
	} else {
		return fmt.Errorf("Invalid trim threshold: %s", args[0])
	}

	if nArgs > 1 && len(args[1]) > 0 {
		if c, err := vips.ColorFromHex(args[1]); err == nil {
			po.Trim.Color = c
			po.Trim.Smart = false
		} else {
			return fmt.Errorf("Invalid trim color: %s", args[1])
		}
	}

	if nArgs > 2 && len(args[2]) > 0 {
		po.Trim.EqualHor = parseBoolOption(args[2])
	}

	if nArgs > 3 && len(args[3]) > 0 {
		po.Trim.EqualVer = parseBoolOption(args[3])
	}

	return nil
}

func applyRotateOption(po *ProcessingOptions, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("Invalid rotate arguments: %v", args)
	}

	if r, err := strconv.Atoi(args[0]); err == nil && r%90 == 0 {
		po.Rotate = r
	} else {
		return fmt.Errorf("Invalid rotation angle: %s", args[0])
	}

	return nil
}

func applyQualityOption(po *ProcessingOptions, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("Invalid quality arguments: %v", args)
	}

	if q, err := strconv.Atoi(args[0]); err == nil && q >= 0 && q <= 100 {
		po.Quality = q
	} else {
		return fmt.Errorf("Invalid quality: %s", args[0])
	}

	return nil
}

func applyMaxBytesOption(po *ProcessingOptions, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("Invalid max_bytes arguments: %v", args)
	}

	if max, err := strconv.Atoi(args[0]); err == nil && max >= 0 {
		po.MaxBytes = max
	} else {
		return fmt.Errorf("Invalid max_bytes: %s", args[0])
	}

	return nil
}

func applyBackgroundOption(po *ProcessingOptions, args []string) error {
	switch len(args) {
	case 1:
		if len(args[0]) == 0 {
			po.Flatten = false
		} else if c, err := vips.ColorFromHex(args[0]); err == nil {
			po.Flatten = true
			po.Background = c
		} else {
			return fmt.Errorf("Invalid background argument: %s", err)
		}

	case 3:
		po.Flatten = true

		if r, err := strconv.ParseUint(args[0], 10, 8); err == nil && r <= 255 {
			po.Background.R = uint8(r)
		} else {
			return fmt.Errorf("Invalid background red channel: %s", args[0])
		}

		if g, err := strconv.ParseUint(args[1], 10, 8); err == nil && g <= 255 {
			po.Background.G = uint8(g)
		} else {
			return fmt.Errorf("Invalid background green channel: %s", args[1])
		}

		if b, err := strconv.ParseUint(args[2], 10, 8); err == nil && b <= 255 {
			po.Background.B = uint8(b)
		} else {
			return fmt.Errorf("Invalid background blue channel: %s", args[2])
		}

	default:
		return fmt.Errorf("Invalid background arguments: %v", args)
	}

	return nil
}

func applyBlurOption(po *ProcessingOptions, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("Invalid blur arguments: %v", args)
	}

	if b, err := strconv.ParseFloat(args[0], 32); err == nil && b >= 0 {
		po.Blur = float32(b)
	} else {
		return fmt.Errorf("Invalid blur: %s", args[0])
	}

	return nil
}

func applySharpenOption(po *ProcessingOptions, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("Invalid sharpen arguments: %v", args)
	}

	if s, err := strconv.ParseFloat(args[0], 32); err == nil && s >= 0 {
		po.Sharpen = float32(s)
	} else {
		return fmt.Errorf("Invalid sharpen: %s", args[0])
	}

	return nil
}

func applyPresetOption(po *ProcessingOptions, args []string) error {
	for _, preset := range args {
		if p, ok := presets[preset]; ok {
			if po.isPresetUsed(preset) {
				log.Warningf("Recursive preset usage is detected: %s", preset)
				continue
			}

			po.UsedPresets = append(po.UsedPresets, preset)

			if err := applyURLOptions(po, p); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("Unknown preset: %s", preset)
		}
	}

	return nil
}

func applyWatermarkOption(po *ProcessingOptions, args []string) error {
	if len(args) > 7 {
		return fmt.Errorf("Invalid watermark arguments: %v", args)
	}

	if o, err := strconv.ParseFloat(args[0], 64); err == nil && o >= 0 && o <= 1 {
		po.Watermark.Enabled = o > 0
		po.Watermark.Opacity = o
	} else {
		return fmt.Errorf("Invalid watermark opacity: %s", args[0])
	}

	if len(args) > 1 && len(args[1]) > 0 {
		if args[1] == "re" {
			po.Watermark.Replicate = true
		} else if g, ok := gravityTypes[args[1]]; ok && g != GravityFocusPoint && g != GravitySmart {
			po.Watermark.Gravity.Type = g
		} else {
			return fmt.Errorf("Invalid watermark position: %s", args[1])
		}
	}

	if len(args) > 2 && len(args[2]) > 0 {
		if x, err := strconv.Atoi(args[2]); err == nil {
			po.Watermark.Gravity.X = float64(x)
		} else {
			return fmt.Errorf("Invalid watermark X offset: %s", args[2])
		}
	}

	if len(args) > 3 && len(args[3]) > 0 {
		if y, err := strconv.Atoi(args[3]); err == nil {
			po.Watermark.Gravity.Y = float64(y)
		} else {
			return fmt.Errorf("Invalid watermark Y offset: %s", args[3])
		}
	}

	if len(args) > 4 && len(args[4]) > 0 {
		if s, err := strconv.ParseFloat(args[4], 64); err == nil && s >= 0 {
			po.Watermark.Scale = s
		} else {
			return fmt.Errorf("Invalid watermark scale: %s", args[4])
		}
	}

	return nil
}

func applyFormatOption(po *ProcessingOptions, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("Invalid format arguments: %v", args)
	}

	if f, ok := imagetype.Types[args[0]]; ok {
		po.Format = f
	} else {
		return fmt.Errorf("Invalid image format: %s", args[0])
	}

	return nil
}

func applyCacheBusterOption(po *ProcessingOptions, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("Invalid cache buster arguments: %v", args)
	}

	po.CacheBuster = args[0]

	return nil
}

func applySkipProcessingFormatsOption(po *ProcessingOptions, args []string) error {
	for _, format := range args {
		if f, ok := imagetype.Types[format]; ok {
			po.SkipProcessingFormats = append(po.SkipProcessingFormats, f)
		} else {
			return fmt.Errorf("Invalid image format in skip processing: %s", format)
		}
	}

	return nil
}

func applyFilenameOption(po *ProcessingOptions, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("Invalid filename arguments: %v", args)
	}

	po.Filename = args[0]

	return nil
}

func applyExpiresOption(po *ProcessingOptions, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("Invalid expires arguments: %v", args)
	}

	timestamp, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("Invalid expires argument: %v", args[0])
	}

	if timestamp > 0 && timestamp < time.Now().Unix() {
		return errExpiredURL
	}

	return nil
}

func applyStripMetadataOption(po *ProcessingOptions, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("Invalid strip metadata arguments: %v", args)
	}

	po.StripMetadata = parseBoolOption(args[0])

	return nil
}

func applyStripColorProfileOption(po *ProcessingOptions, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("Invalid strip color profile arguments: %v", args)
	}

	po.StripColorProfile = parseBoolOption(args[0])

	return nil
}

func applyAutoRotateOption(po *ProcessingOptions, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("Invalid auto rotate arguments: %v", args)
	}

	po.AutoRotate = parseBoolOption(args[0])

	return nil
}

func applyURLOption(po *ProcessingOptions, name string, args []string) error {
	switch name {
	case "resize", "rs":
		return applyResizeOption(po, args)
	case "size", "s":
		return applySizeOption(po, args)
	case "resizing_type", "rt":
		return applyResizingTypeOption(po, args)
	case "width", "w":
		return applyWidthOption(po, args)
	case "height", "h":
		return applyHeightOption(po, args)
	case "min-width", "mw":
		return applyMinWidthOption(po, args)
	case "min-height", "mh":
		return applyMinHeightOption(po, args)
	case "dpr":
		return applyDprOption(po, args)
	case "enlarge", "el":
		return applyEnlargeOption(po, args)
	case "extend", "ex":
		return applyExtendOption(po, args)
	case "gravity", "g":
		return applyGravityOption(po, args)
	case "crop", "c":
		return applyCropOption(po, args)
	case "trim", "t":
		return applyTrimOption(po, args)
	case "padding", "pd":
		return applyPaddingOption(po, args)
	case "auto_rotate", "ar":
		return applyAutoRotateOption(po, args)
	case "rotate", "rot":
		return applyRotateOption(po, args)
	case "background", "bg":
		return applyBackgroundOption(po, args)
	case "blur", "bl":
		return applyBlurOption(po, args)
	case "sharpen", "sh":
		return applySharpenOption(po, args)
	case "watermark", "wm":
		return applyWatermarkOption(po, args)
	case "strip_metadata", "sm":
		return applyStripMetadataOption(po, args)
	case "strip_color_profile", "scp":
		return applyStripColorProfileOption(po, args)
	// Saving options
	case "quality", "q":
		return applyQualityOption(po, args)
	case "max_bytes", "mb":
		return applyMaxBytesOption(po, args)
	case "format", "f", "ext":
		return applyFormatOption(po, args)
	// Handling options
	case "skip_processing", "skp":
		return applySkipProcessingFormatsOption(po, args)
	case "cachebuster", "cb":
		return applyCacheBusterOption(po, args)
	case "expires", "exp":
		return applyExpiresOption(po, args)
	case "filename", "fn":
		return applyFilenameOption(po, args)
	// Presets
	case "preset", "pr":
		return applyPresetOption(po, args)
	}

	return fmt.Errorf("Unknown processing option: %s", name)
}

func applyURLOptions(po *ProcessingOptions, options urlOptions) error {
	for _, opt := range options {
		if err := applyURLOption(po, opt.Name, opt.Args); err != nil {
			return err
		}
	}

	return nil
}

func defaultProcessingOptions(headers http.Header) (*ProcessingOptions, error) {
	po := NewProcessingOptions()

	headerAccept := headers.Get("Accept")

	if strings.Contains(headerAccept, "image/webp") {
		po.PreferWebP = config.EnableWebpDetection || config.EnforceWebp
		po.EnforceWebP = config.EnforceWebp
	}

	if strings.Contains(headerAccept, "image/avif") {
		po.PreferAvif = config.EnableAvifDetection || config.EnforceAvif
		po.EnforceAvif = config.EnforceAvif
	}

	if config.EnableClientHints {
		if headerViewportWidth := headers.Get("Viewport-Width"); len(headerViewportWidth) > 0 {
			if vw, err := strconv.Atoi(headerViewportWidth); err == nil {
				po.Width = vw
			}
		}
		if headerWidth := headers.Get("Width"); len(headerWidth) > 0 {
			if w, err := strconv.Atoi(headerWidth); err == nil {
				po.Width = w
			}
		}
		if headerDPR := headers.Get("DPR"); len(headerDPR) > 0 {
			if dpr, err := strconv.ParseFloat(headerDPR, 64); err == nil && (dpr > 0 && dpr <= maxClientHintDPR) {
				po.Dpr = dpr
			}
		}
	}

	if _, ok := presets["default"]; ok {
		if err := applyPresetOption(po, []string{"default"}); err != nil {
			return po, err
		}
	}

	return po, nil
}

func parsePathOptions(parts []string, headers http.Header) (*ProcessingOptions, string, error) {
	po, err := defaultProcessingOptions(headers)
	if err != nil {
		return nil, "", err
	}

	options, urlParts := parseURLOptions(parts)

	if err = applyURLOptions(po, options); err != nil {
		return nil, "", err
	}

	url, extension, err := DecodeURL(urlParts)
	if err != nil {
		return nil, "", err
	}

	if len(extension) > 0 {
		if err = applyFormatOption(po, []string{extension}); err != nil {
			return nil, "", err
		}
	}

	return po, url, nil
}

func parsePathPresets(parts []string, headers http.Header) (*ProcessingOptions, string, error) {
	po, err := defaultProcessingOptions(headers)
	if err != nil {
		return nil, "", err
	}

	presets := strings.Split(parts[0], ":")
	urlParts := parts[1:]

	if err = applyPresetOption(po, presets); err != nil {
		return nil, "", err
	}

	url, extension, err := DecodeURL(urlParts)
	if err != nil {
		return nil, "", err
	}

	if len(extension) > 0 {
		if err = applyFormatOption(po, []string{extension}); err != nil {
			return nil, "", err
		}
	}

	return po, url, nil
}

func ParsePath(path string, headers http.Header) (*ProcessingOptions, string, error) {
	if path == "" || path == "/" {
		return nil, "", ierrors.New(404, fmt.Sprintf("Invalid path: %s", path), "Invalid URL")
	}

	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")

	var (
		imageURL string
		po       *ProcessingOptions
		err      error
	)

	if config.OnlyPresets {
		po, imageURL, err = parsePathPresets(parts, headers)
	} else {
		po, imageURL, err = parsePathOptions(parts, headers)
	}

	if err != nil {
		return nil, "", ierrors.New(404, err.Error(), "Invalid URL")
	}

	return po, imageURL, nil
}