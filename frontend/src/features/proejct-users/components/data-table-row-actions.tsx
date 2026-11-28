import { DotsHorizontalIcon } from '@radix-ui/react-icons';
import { Row } from '@tanstack/react-table';
import { IconEdit, IconUserCheck, IconUserMinus, IconUserOff } from '@tabler/icons-react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '@/hooks/usePermissions';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useUsers } from '../context/users-context';
import { User } from '../data/schema';

interface DataTableRowActionsProps {
  row: Row<User>;
}

export function DataTableRowActions({ row }: DataTableRowActionsProps) {
  const { t } = useTranslation();
  const { setOpen, setCurrentRow } = useUsers();
  const { userPermissions, isProjectOwner, user: currentUser } = usePermissions();
  const member = row.original;
  // Project owners may deactivate/reactivate ordinary members' accounts, but
  // never their own account or another owner's.
  const canToggleAccountStatus = isProjectOwner && member.id !== currentUser?.id && !member.isOwner;

  // Don't show menu if user has no write permissions
  if (!userPermissions.canWrite) {
    return null;
  }

  return (
    <>
      <DropdownMenu modal={false}>
        <DropdownMenuTrigger asChild>
          <Button variant='ghost' className='data-[state=open]:bg-muted flex h-8 w-8 p-0'>
            <DotsHorizontalIcon className='h-4 w-4' />
            <span className='sr-only'>{t('common.actions.openMenu')}</span>
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align='end' className='w-[180px]'>
          {/* Edit - requires write permission */}
          {userPermissions.canEdit && (
            <DropdownMenuItem
              onClick={() => {
                setCurrentRow(row.original);
                setOpen('edit');
              }}
            >
              <IconEdit size={16} className='mr-2' />
              {t('common.actions.edit')}
            </DropdownMenuItem>
          )}

          {canToggleAccountStatus && (
            <DropdownMenuItem
              onClick={() => {
                setCurrentRow(member);
                setOpen('status');
              }}
              className={member.status === 'activated' ? 'text-orange-600' : 'text-green-600'}
            >
              {member.status === 'activated' ? (
                <IconUserOff size={16} className='mr-2' />
              ) : (
                <IconUserCheck size={16} className='mr-2' />
              )}
              {member.status === 'activated' ? t('users.actions.deactivateAccount') : t('users.actions.reactivateAccount')}
            </DropdownMenuItem>
          )}

          {/* Separator only if there are both edit and remove actions */}
          {userPermissions.canEdit && userPermissions.canWrite && <DropdownMenuSeparator />}

          {/* Remove from project - requires write permission */}
          {userPermissions.canWrite && (
            <DropdownMenuItem
              onClick={() => {
                setCurrentRow(row.original);
                setOpen('remove');
              }}
              className='text-red-500!'
            >
              <IconUserMinus size={16} className='mr-2' />
              {t('users.actions.removeFromProject')}
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </>
  );
}
