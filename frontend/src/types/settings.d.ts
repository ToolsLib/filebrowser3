interface ISettings {
  downloadRate?: number | null;
  maxUploadSize?: number;
  uploadLimits?: UploadLimits;
  signup: boolean;
  createUserDir: boolean;
  hideLoginButton: boolean;
  minimumPasswordLength: number;
  userHomeBasePath: string;
  defaults: SettingsDefaults;
  authMethod: string;
  rules: any[];
  branding: SettingsBranding;
  tus: SettingsTus;
  shell: string[];
  commands: SettingsCommand;
}

interface SettingsDefaults {
  downloadRate?: number | null;
  maxUploadSize?: number;
  scope: string;
  locale: string;
  viewMode: ViewModeType;
  singleClick: boolean;
  redirectAfterCopyMove: boolean;
  sorting: Sorting;
  perm: Permissions;
  commands: any[];
  hideDotfiles: boolean;
  dateFormat: boolean;
  aceEditorTheme: string;
}

interface UploadLimits {
  deploymentLimit: number;
  deploymentSource: string;
  defaultLimit: number;
  globalLimit: number;
}

interface SettingsBranding {
  name: string;
  disableExternal: boolean;
  disableUsedPercentage: boolean;
  files: string;
  theme: UserTheme;
  color: string;
}

interface SettingsTus {
  chunkSize: number;
  retryCount: number;
}

interface SettingsCommand {
  after_copy?: string[];
  after_delete?: string[];
  after_rename?: string[];
  after_save?: string[];
  after_upload?: string[];
  before_copy?: string[];
  before_delete?: string[];
  before_rename?: string[];
  before_save?: string[];
  before_upload?: string[];
}

interface SettingsUnit {
  KB: number;
  MB: number;
  GB: number;
  TB: number;
}
