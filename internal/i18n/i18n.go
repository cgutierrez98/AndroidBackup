package i18n

// Language represents a supported language
type Language string

const (
	English Language = "en"
	Spanish Language = "es"
)

// Strings holds all translatable strings
type Strings struct {
	// App
	AppTitle string

	// Device Status
	DeviceStatus    string
	AndroidDevice   string
	CheckingConn    string
	Connected       string
	NoDevice        string
	ADBNotFound     string
	ADBError        string
	WaitingDevice   string
	DeviceConnected string

	// Configuration
	Configuration string
	SourcePath    string
	DestPath      string
	QuickSelect   string
	FileTypes     string
	IncludeDocs   string
	Language      string

	// Actions
	Actions         string
	ScanFiles       string
	StartBackup     string
	GenerateGallery string
	Restore         string

	// Log & Progress
	ActivityLog       string
	Scanning          string
	FoundFiles        string
	ScanFailed        string
	StartingBackup    string
	LoadingIndex      string
	RegistryWarning   string
	Finished          string
	ManifestSaved     string
	ManifestSaveFail  string
	GeneratingGallery string
	GalleryCreated    string
	GalleryError      string
	GalleryIncomplete string
	NoMediaFiles      string

	// Restore
	ConfirmRestore      string
	RestoreNow          string
	Cancel              string
	NoManifestMsg       string
	ManifestFoundMsg    string
	RestoreToOriginal   string
	RestoreCancelled    string
	RestoringTo         string
	RestoringToOriginal string
	RestoreComplete     string
	RestoreFailed       string
	Progress            string

	// File status
	Fail string
	Skip string
	OK   string

	// Validation
	InfoTitle  string
	ScanFirst  string

	// Resume backup
	ResumeTitle   string
	ResumeMsg     string
	ResumeYes     string
	ResumeNo      string
	Resuming      string

	// Tabs
	TabBackup  string
	TabGallery string
	TabRestore string

	// Status bar
	StatusReady    string
	StatusWorkers  string
	StatusSpeed    string
	StatusFreeGB   string
	StatusFreeErr  string

	// Pre-backup stats (E1)
	StatsNew     string
	StatsSkipped string
	StatsMB      string

	// Workers slider (F1)
	WorkersLabel string

	// Notification (F3)
	NotifyTitle   string
	NotifyBackup  string
	NotifyGallery string

	// Profiles (F4)
	ProfileLabel  string
	ProfileSave   string
	ProfileSaved  string
	ProfileLoad   string
	ProfileNew    string

	// Gallery open in browser (F5)
	OpenInBrowser string

	// Exclusions (E2)
	ExcludeLabel        string
	ExcludePresets      string
	ExcludeCustom       string
	ExcludePlaceholder  string
}

var currentLang Language = English

var translations = map[Language]Strings{
	English: {
		AppTitle:        "AndroidSafeLocal",
		DeviceStatus:    "Device Status",
		AndroidDevice:   "Android Device",
		CheckingConn:    "Checking connection...",
		Connected:       "Connected",
		NoDevice:        "No Device Connected.\nCheck USB Cable.",
		ADBNotFound:     "Error: ADB not found",
		ADBError:        "ADB Error",
		WaitingDevice:   "Waiting for device...",
		DeviceConnected: "Device connected",

		Configuration: "Configuration",
		SourcePath:    "Source Path (Mobile)",
		DestPath:      "Destination Path (PC)",
		QuickSelect:   "Quick Select...",
		FileTypes:     "File Types",
		IncludeDocs:   "Include Documents (PDF, Word, Excel)",
		Language:      "Language",

		Actions:         "Actions",
		ScanFiles:       "Scan Files",
		StartBackup:     "Start Backup",
		GenerateGallery: "Generate Gallery",
		Restore:         "Restore",

		ActivityLog:       "Activity Log",
		Scanning:          "Scanning",
		FoundFiles:        "Found %d files.",
		ScanFailed:        "Scan failed",
		StartingBackup:    "Starting backup...",
		LoadingIndex:      "Loading local index...",
		RegistryWarning:   "Registry warning",
		Finished:          "Finished. Processed: %d. Failures: %d",
		ManifestSaved:     "Manifest saved.",
		ManifestSaveFail:  "Warning: Failed to save manifest",
		GeneratingGallery: "Generating Gallery...",
		GalleryCreated:    "Gallery Created! (%d items)",
		GalleryError:      "Gallery Error",
		GalleryIncomplete: "Gallery incomplete (%d items). Error: %s",
		NoMediaFiles:      "No media files found.",

		ConfirmRestore:      "Confirm Restore",
		RestoreNow:          "Restore Now",
		Cancel:              "Cancel",
		NoManifestMsg:       "No manifest found.\nRestore entire folder to:\n%s\n\nExisting files may be overwritten.",
		ManifestFoundMsg:    "Manifest found with %d files.\nRestore each file to its ORIGINAL location on the device?\n\nExisting files with same name will be overwritten.",
		RestoreToOriginal:   "Restore to Original",
		RestoreCancelled:    "Restore cancelled.",
		RestoringTo:         "Restoring folder to",
		RestoringToOriginal: "Restoring to original locations...",
		RestoreComplete:     "Restore Complete. Success: %d, Failures: %d",
		RestoreFailed:       "Restore failed",
		Progress:            "Progress: %d/%d files restored...",

		Fail: "FAIL",
		Skip: "SKIP",
		OK:   "OK",

		InfoTitle: "Info",
		ScanFirst: "Please scan for files first.",

		ResumeTitle: "Resume Backup",
		ResumeMsg:   "A previous backup was interrupted.\nResume from where it stopped?",
		ResumeYes:   "Resume",
		ResumeNo:    "Start Fresh",
		Resuming:    "Resuming previous backup...",

		TabBackup:  "Backup",
		TabGallery: "Gallery",
		TabRestore: "Restore",

		StatusReady:   "Ready",
		StatusWorkers: "Workers: %d",
		StatusSpeed:   "%.1f MB/s",
		StatusFreeGB:  "Free: %.1f GB",
		StatusFreeErr: "Free: —",

		StatsNew:     "New: %d",
		StatsSkipped: "Already backed up: %d",
		StatsMB:      "Pending: %.1f MB",

		WorkersLabel: "Concurrent Workers",

		NotifyTitle:   "AndroidSafeLocal",
		NotifyBackup:  "Backup complete. %d files transferred.",
		NotifyGallery: "Gallery generated. %d items.",

		ProfileLabel: "Profile",
		ProfileSave:  "Save Profile",
		ProfileSaved: "Profile saved.",
		ProfileLoad:  "Load Profile",
		ProfileNew:   "New Profile...",

		OpenInBrowser: "Open in browser when done",

		ExcludeLabel:       "Exclusions",
		ExcludePresets:     "Common presets",
		ExcludeCustom:      "Custom patterns:",
		ExcludePlaceholder: ".tmp, vacation/",
	},
	Spanish: {
		AppTitle:        "AndroidSafeLocal",
		DeviceStatus:    "Estado del Dispositivo",
		AndroidDevice:   "Dispositivo Android",
		CheckingConn:    "Comprobando conexión...",
		Connected:       "Conectado",
		NoDevice:        "Sin Dispositivo.\nRevisa el cable USB.",
		ADBNotFound:     "Error: ADB no encontrado",
		ADBError:        "Error ADB",
		WaitingDevice:   "Esperando dispositivo...",
		DeviceConnected: "Dispositivo conectado",

		Configuration: "Configuración",
		SourcePath:    "Ruta Origen (Móvil)",
		DestPath:      "Ruta Destino (PC)",
		QuickSelect:   "Selección Rápida...",
		FileTypes:     "Tipos de Archivo",
		IncludeDocs:   "Incluir Documentos (PDF, Word, Excel)",
		Language:      "Idioma",

		Actions:         "Acciones",
		ScanFiles:       "Escanear",
		StartBackup:     "Iniciar Backup",
		GenerateGallery: "Generar Galería",
		Restore:         "Restaurar",

		ActivityLog:       "Registro de Actividad",
		Scanning:          "Escaneando",
		FoundFiles:        "Encontrados %d archivos.",
		ScanFailed:        "Escaneo fallido",
		StartingBackup:    "Iniciando backup...",
		LoadingIndex:      "Cargando índice local...",
		RegistryWarning:   "Aviso del registro",
		Finished:          "Finalizado. Procesados: %d. Fallos: %d",
		ManifestSaved:     "Manifiesto guardado.",
		ManifestSaveFail:  "Aviso: Error al guardar manifiesto",
		GeneratingGallery: "Generando Galería...",
		GalleryCreated:    "¡Galería Creada! (%d elementos)",
		GalleryError:      "Error de Galería",
		GalleryIncomplete: "Galería incompleta (%d elementos). Error: %s",
		NoMediaFiles:      "No se encontraron archivos multimedia.",

		ConfirmRestore:      "Confirmar Restauración",
		RestoreNow:          "Restaurar Ahora",
		Cancel:              "Cancelar",
		NoManifestMsg:       "No se encontró manifiesto.\nRestaurar carpeta completa a:\n%s\n\nLos archivos existentes pueden sobrescribirse.",
		ManifestFoundMsg:    "Manifiesto encontrado con %d archivos.\n¿Restaurar cada archivo a su ubicación ORIGINAL en el dispositivo?\n\nLos archivos existentes se sobrescribirán.",
		RestoreToOriginal:   "Restaurar a Original",
		RestoreCancelled:    "Restauración cancelada.",
		RestoringTo:         "Restaurando carpeta a",
		RestoringToOriginal: "Restaurando a ubicaciones originales...",
		RestoreComplete:     "Restauración Completa. Éxito: %d, Fallos: %d",
		RestoreFailed:       "Restauración fallida",
		Progress:            "Progreso: %d/%d archivos restaurados...",

		Fail: "FALLO",
		Skip: "OMITIDO",
		OK:   "OK",

		InfoTitle: "Info",
		ScanFirst: "Por favor, escanea los archivos primero.",

		ResumeTitle: "Reanudar Backup",
		ResumeMsg:   "El backup anterior fue interrumpido.\n¿Reanudar desde donde se detuvo?",
		ResumeYes:   "Reanudar",
		ResumeNo:    "Empezar de Nuevo",
		Resuming:    "Reanudando backup anterior...",

		TabBackup:  "Backup",
		TabGallery: "Galería",
		TabRestore: "Restaurar",

		StatusReady:   "Listo",
		StatusWorkers: "Workers: %d",
		StatusSpeed:   "%.1f MB/s",
		StatusFreeGB:  "Libre: %.1f GB",
		StatusFreeErr: "Libre: —",

		StatsNew:     "Nuevos: %d",
		StatsSkipped: "Ya respaldados: %d",
		StatsMB:      "Pendiente: %.1f MB",

		WorkersLabel: "Workers concurrentes",

		NotifyTitle:   "AndroidSafeLocal",
		NotifyBackup:  "Backup completado. %d archivos transferidos.",
		NotifyGallery: "Galería generada. %d elementos.",

		ProfileLabel: "Perfil",
		ProfileSave:  "Guardar Perfil",
		ProfileSaved: "Perfil guardado.",
		ProfileLoad:  "Cargar Perfil",
		ProfileNew:   "Nuevo perfil...",

		OpenInBrowser: "Abrir en navegador al terminar",

		ExcludeLabel:       "Exclusiones",
		ExcludePresets:     "Presets comunes",
		ExcludeCustom:      "Patrones personalizados:",
		ExcludePlaceholder: ".tmp, vacaciones/",
	},
}

// SetLanguage changes the current language
func SetLanguage(lang Language) {
	currentLang = lang
}

// GetLanguage returns the current language
func GetLanguage() Language {
	return currentLang
}

// T returns the current translations
func T() Strings {
	return translations[currentLang]
}

// LanguageNames returns display names for languages
func LanguageNames() map[Language]string {
	return map[Language]string{
		English: "English",
		Spanish: "Español",
	}
}
