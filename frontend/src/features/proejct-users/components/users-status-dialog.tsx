// 职责: 项目 Owner 在成员列表中确认停用或恢复成员账号。
// 关联文件:
//   frontend/src/features/proejct-users/data/users.ts
//   frontend/src/features/proejct-users/components/users-dialogs.tsx
//   frontend/src/features/proejct-users/components/data-table-row-actions.tsx
//   frontend/src/locales/en/users.json
//   frontend/src/locales/zh-CN/users.json
// 遵循的 Rules:
//   .agents/rules/frontend-general.md
//   .agents/rules/frontend-i18n.md
// 注意事项:
//   停用的是整个用户账号，不只是项目成员关系；成员还属于调用者不拥有的项目时后端会拒绝，错误由 mutation 的错误处理展示。
import { IconUserCheck, IconUserOff } from '@tabler/icons-react';
import { useTranslation } from 'react-i18next';
import { formatUserName } from '@/lib/utils';
import { ConfirmDialog } from '@/components/confirm-dialog';
import { User } from '../data/schema';
import { useUpdateUserStatus } from '../data/users';

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  currentRow: User;
}

export function UsersStatusDialog({ open, onOpenChange, currentRow }: Props) {
  const { t } = useTranslation();
  const updateUserStatus = useUpdateUserStatus();
  const isActivated = currentRow.status === 'activated';
  const name = formatUserName(currentRow.firstName, currentRow.lastName) || currentRow.email;

  const handleConfirm = async () => {
    try {
      await updateUserStatus.mutateAsync({
        id: currentRow.id,
        status: isActivated ? 'deactivated' : 'activated',
      });
      onOpenChange(false);
    } catch {
      // Errors are surfaced by the mutation's error handler.
    }
  };

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      handleConfirm={handleConfirm}
      disabled={updateUserStatus.isPending}
      title={
        <span className={isActivated ? 'text-destructive' : 'text-green-600'}>
          {isActivated ? <IconUserOff className='mr-1 inline-block' size={18} /> : <IconUserCheck className='mr-1 inline-block' size={18} />}
          {isActivated ? t('users.dialogs.accountStatus.deactivateTitle') : t('users.dialogs.accountStatus.reactivateTitle')}
        </span>
      }
      desc={
        <div className='space-y-2'>
          <p>
            {isActivated
              ? t('users.dialogs.accountStatus.deactivateConfirm', { name })
              : t('users.dialogs.accountStatus.reactivateConfirm', { name })}
          </p>
          <p className='text-muted-foreground text-sm'>
            {isActivated ? t('users.dialogs.accountStatus.deactivateWarning') : t('users.dialogs.accountStatus.reactivateInfo')}
          </p>
        </div>
      }
      confirmText={isActivated ? t('users.actions.deactivateAccount') : t('users.actions.reactivateAccount')}
      cancelBtnText={t('common.buttons.cancel')}
      destructive={isActivated}
    />
  );
}
