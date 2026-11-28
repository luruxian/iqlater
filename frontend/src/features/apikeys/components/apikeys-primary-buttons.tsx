import { IconKey, IconPlus, IconTemplate } from '@tabler/icons-react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '@/hooks/usePermissions';
import { Button } from '@/components/ui/button';
import { PermissionGuard } from '@/components/permission-guard';
import { useApiKeysContext } from '../context/apikeys-context';

export function ApiKeysPrimaryButtons() {
  const { t } = useTranslation();
  const { openDialog } = useApiKeysContext();
  const { isProjectOwner } = usePermissions();

  return (
    <div className='flex min-w-0 max-w-full flex-wrap items-center gap-2'>
      <PermissionGuard requiredScope='write_api_keys'>
        <Button variant='outline' size='sm' onClick={() => openDialog('profileTemplates')}>
          <IconTemplate className='mr-2 h-4 w-4' />
          {t('apikeys.profileTemplates.button')}
        </Button>
        {isProjectOwner && (
          <Button variant='outline' size='sm' onClick={() => openDialog('issueForMember')}>
            <IconKey className='mr-2 h-4 w-4' />
            {t('apikeys.issueForMember')}
          </Button>
        )}
        <Button onClick={() => openDialog('create')} size='sm'>
          <IconPlus className='mr-2 h-4 w-4' />
          {t('apikeys.createApiKey')}
        </Button>
      </PermissionGuard>
    </div>
  );
}
