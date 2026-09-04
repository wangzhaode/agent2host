package model

// AndroidTarget is an Android device or emulator visible to the adb server on a host.
type AndroidTarget struct {
	Host        string            `json:"host"`
	Serial      string            `json:"serial"`
	State       string            `json:"state"`
	Product     string            `json:"product,omitempty"`
	Model       string            `json:"model,omitempty"`
	Codename    string            `json:"codename,omitempty"`
	USB         string            `json:"usb,omitempty"`
	TransportID string            `json:"transport_id,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty"`
}

type AndroidListRequest struct {
	Host      string `json:"host" jsonschema:"host on which adb runs; use local for this machine"`
	TimeoutMS int    `json:"timeout_ms,omitempty" jsonschema:"timeout in milliseconds; defaults to 10000"`
}

type AndroidExecRequest struct {
	Host      string `json:"host" jsonschema:"host on which adb runs; use local for this machine"`
	Serial    string `json:"serial" jsonschema:"exact serial returned by android_list"`
	Command   string `json:"command" jsonschema:"shell command to execute on the Android target"`
	TimeoutMS int    `json:"timeout_ms,omitempty" jsonschema:"timeout in milliseconds; defaults to 30000"`
}

type AndroidOpenSessionRequest struct {
	Host   string `json:"host" jsonschema:"host on which adb runs; use local for this machine"`
	Serial string `json:"serial" jsonschema:"exact serial returned by android_list"`
	Cols   uint16 `json:"cols,omitempty" jsonschema:"PTY width; defaults to 120"`
	Rows   uint16 `json:"rows,omitempty" jsonschema:"PTY height; defaults to 40"`
}

type AndroidPushRequest struct {
	Host        string `json:"host" jsonschema:"host on which adb runs; use local for this machine"`
	Serial      string `json:"serial" jsonschema:"exact serial returned by android_list"`
	LocalPath   string `json:"local_path" jsonschema:"source file path on the Agent2Host machine"`
	AndroidPath string `json:"android_path" jsonschema:"destination path on the Android target"`
	TimeoutMS   int    `json:"timeout_ms,omitempty" jsonschema:"timeout in milliseconds; defaults to 300000"`
}

type AndroidPullRequest struct {
	Host         string `json:"host" jsonschema:"host on which adb runs; use local for this machine"`
	Serial       string `json:"serial" jsonschema:"exact serial returned by android_list"`
	AndroidPath  string `json:"android_path" jsonschema:"source file path on the Android target"`
	LocalPath    string `json:"local_path" jsonschema:"destination file path on the Agent2Host machine"`
	CreateParent bool   `json:"create_parent,omitempty" jsonschema:"create missing local parent directories"`
	TimeoutMS    int    `json:"timeout_ms,omitempty" jsonschema:"timeout in milliseconds; defaults to 300000"`
}

type AndroidInstallRequest struct {
	Host      string `json:"host" jsonschema:"host on which adb runs; use local for this machine"`
	Serial    string `json:"serial" jsonschema:"exact serial returned by android_list"`
	LocalPath string `json:"local_path" jsonschema:"APK path on the Agent2Host machine"`
	Replace   bool   `json:"replace,omitempty" jsonschema:"replace an already installed application"`
	GrantAll  bool   `json:"grant_all,omitempty" jsonschema:"grant runtime permissions requested by the application"`
	TestOnly  bool   `json:"test_only,omitempty" jsonschema:"allow installation of a test-only APK"`
	TimeoutMS int    `json:"timeout_ms,omitempty" jsonschema:"timeout in milliseconds; defaults to 300000"`
}

type AndroidLogcatRequest struct {
	Host      string   `json:"host" jsonschema:"host on which adb runs; use local for this machine"`
	Serial    string   `json:"serial" jsonschema:"exact serial returned by android_list"`
	Lines     int      `json:"lines,omitempty" jsonschema:"number of recent lines to return; defaults to 200 and is capped at 100000"`
	Filters   []string `json:"filters,omitempty" jsonschema:"optional logcat filter specifications such as ActivityManager:I or *:S"`
	TimeoutMS int      `json:"timeout_ms,omitempty" jsonschema:"timeout in milliseconds; defaults to 30000"`
}

type AndroidScreenshotRequest struct {
	Host         string `json:"host" jsonschema:"host on which adb runs; use local for this machine"`
	Serial       string `json:"serial" jsonschema:"exact serial returned by android_list"`
	LocalPath    string `json:"local_path" jsonschema:"destination PNG path on the Agent2Host machine"`
	CreateParent bool   `json:"create_parent,omitempty" jsonschema:"create missing local parent directories"`
	TimeoutMS    int    `json:"timeout_ms,omitempty" jsonschema:"timeout in milliseconds; defaults to 30000"`
}

type AndroidTransferResult struct {
	Output     string `json:"output,omitempty"`
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"`
	TimedOut   bool   `json:"timed_out,omitempty"`
	Bytes      int64  `json:"bytes,omitempty"`
	LocalPath  string `json:"local_path,omitempty"`
}
