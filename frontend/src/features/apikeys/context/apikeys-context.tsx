import { createContext, useContext, useState, useRef } from 'react';
import { ApiKey } from '../data/schema';

type ApiKeyDialogType =
  | 'create'
  | 'edit'
  | 'delete'
  | 'status'
  | 'view'
  | 'profiles'
  | 'profileTemplates'
  | 'archive'
  | 'bulkDisable'
  | 'bulkArchive'
  | 'bulkEnable'
  | 'rotate'
  | 'issueForMember';

interface ApiKeysContextType {
  selectedApiKey: ApiKey | null;
  setSelectedApiKey: (apiKey: ApiKey | null) => void;
  selectedApiKeys: ApiKey[];
  setSelectedApiKeys: (apiKeys: ApiKey[]) => void;
  // True while the view dialog shows a plaintext key that the server returns only once.
  oneTimeKeyNotice: boolean;
  setOneTimeKeyNotice: (value: boolean) => void;
  isDialogOpen: Record<ApiKeyDialogType, boolean>;
  openDialog: (type: ApiKeyDialogType, apiKey?: ApiKey | ApiKey[]) => void;
  closeDialog: (type?: ApiKeyDialogType) => void;
  resetRowSelection: () => void;
  setResetRowSelection: (fn: () => void) => void;
}

const ApiKeysContext = createContext<ApiKeysContextType | undefined>(undefined);

export function ApiKeysProvider({ children }: { children: React.ReactNode }) {
  const [selectedApiKey, setSelectedApiKey] = useState<ApiKey | null>(null);
  const [selectedApiKeys, setSelectedApiKeys] = useState<ApiKey[]>([]);
  const [oneTimeKeyNotice, setOneTimeKeyNotice] = useState(false);
  const [isDialogOpen, setIsDialogOpen] = useState<Record<ApiKeyDialogType, boolean>>({
    create: false,
    edit: false,
    delete: false,
    status: false,
    view: false,
    profiles: false,
    profileTemplates: false,
    archive: false,
    bulkDisable: false,
    bulkArchive: false,
    bulkEnable: false,
    rotate: false,
    issueForMember: false,
  });
  const resetRowSelectionRef = useRef<() => void>(() => {});

  const openDialog = (type: ApiKeyDialogType, apiKey?: ApiKey | ApiKey[]) => {
    if (apiKey) {
      if (Array.isArray(apiKey)) {
        setSelectedApiKeys(apiKey);
      } else {
        setSelectedApiKey(apiKey);
      }
    }
    setIsDialogOpen((prev) => ({ ...prev, [type]: true }));
  };

  const closeDialog = (type?: ApiKeyDialogType) => {
    if (type) {
      setIsDialogOpen((prev) => ({ ...prev, [type]: false }));
      if (type === 'delete' || type === 'edit' || type === 'view' || type === 'archive' || type === 'status' || type === 'profiles' || type === 'rotate') {
        setSelectedApiKey(null);
      }
      if (type === 'view') {
        setOneTimeKeyNotice(false);
      }
      if (type === 'bulkDisable' || type === 'bulkArchive' || type === 'bulkEnable') {
        setSelectedApiKeys([]);
      }
    } else {
      // Close all dialogs
      setIsDialogOpen({
        create: false,
        edit: false,
        delete: false,
        status: false,
        view: false,
        profiles: false,
        profileTemplates: false,
        archive: false,
        bulkDisable: false,
        bulkArchive: false,
        bulkEnable: false,
        rotate: false,
        issueForMember: false,
      });
      setSelectedApiKey(null);
      setSelectedApiKeys([]);
      setOneTimeKeyNotice(false);
    }
  };

  return (
    <ApiKeysContext.Provider
      value={{
        selectedApiKey,
        setSelectedApiKey,
        selectedApiKeys,
        setSelectedApiKeys,
        oneTimeKeyNotice,
        setOneTimeKeyNotice,
        isDialogOpen,
        openDialog,
        closeDialog,
        resetRowSelection: () => resetRowSelectionRef.current(),
        setResetRowSelection: (fn: () => void) => {
          resetRowSelectionRef.current = fn;
        },
      }}
    >
      {children}
    </ApiKeysContext.Provider>
  );
}

export default ApiKeysProvider;

export function useApiKeysContext() {
  const context = useContext(ApiKeysContext);
  if (context === undefined) {
    throw new Error('useApiKeysContext must be used within a ApiKeysProvider');
  }
  return context;
}
