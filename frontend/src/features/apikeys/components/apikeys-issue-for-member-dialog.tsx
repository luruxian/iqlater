// 职责: 项目 Owner 为项目成员签发个人 API Key，成功后用查看弹窗展示仅返回一次的明文 Key。
// 关联文件:
//   frontend/src/features/apikeys/data/apikeys.ts
//   frontend/src/features/apikeys/context/apikeys-context.tsx
//   frontend/src/features/apikeys/components/apikeys-view-dialog.tsx
//   frontend/src/features/proejct-users/data/users.ts
//   frontend/src/locales/en/apikeys.json
//   frontend/src/locales/zh-CN/apikeys.json
// 遵循的 Rules:
//   .agents/rules/frontend-general.md
//   .agents/rules/frontend-ui.md
//   .agents/rules/frontend-i18n.md
// 注意事项:
//   createPersonalAPIKeyForUser 只在本次响应里返回明文 key，必须直接交给查看弹窗展示，不能依赖列表刷新取回。
import { useMemo, useState } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { formatUserName } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Form, FormControl, FormField, FormItem, FormLabel, FormMessage } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { AutoCompleteSelect } from '@/components/auto-complete-select';
import { useUsers as useProjectUsers } from '@/features/proejct-users/data/users';
import { useApiKeysContext } from '../context/apikeys-context';
import { useCreatePersonalApiKeyForUser } from '../data/apikeys';

export function ApiKeysIssueForMemberDialog() {
  const { t } = useTranslation();
  const { isDialogOpen, closeDialog, openDialog, setSelectedApiKey, setOneTimeKeyNotice } = useApiKeysContext();
  const open = isDialogOpen.issueForMember;
  const issueKey = useCreatePersonalApiKeyForUser();
  const { data: projectUsers, isLoading: isLoadingUsers } = useProjectUsers(undefined, { disableAutoFetch: !open });
  const [dialogContent, setDialogContent] = useState<HTMLDivElement | null>(null);

  const formSchema = useMemo(
    () =>
      z.object({
        userID: z.string().min(1, t('apikeys.dialogs.issueForMember.memberRequired')),
        name: z.string().trim().min(1, t('apikeys.validation.nameRequired')),
      }),
    [t]
  );
  type FormValues = z.infer<typeof formSchema>;

  const form = useForm<FormValues>({
    resolver: zodResolver(formSchema),
    defaultValues: { userID: '', name: '' },
  });

  const memberOptions = useMemo(
    () =>
      (projectUsers?.edges ?? [])
        .map((edge) => edge.node)
        .filter((user) => user.status === 'activated')
        .map((user) => {
          const name = formatUserName(user.firstName, user.lastName);
          return { value: user.id, label: name ? `${name} (${user.email})` : user.email };
        }),
    [projectUsers]
  );

  const handleClose = () => {
    form.reset();
    closeDialog('issueForMember');
  };

  const onSubmit = async (values: FormValues) => {
    try {
      const apiKey = await issueKey.mutateAsync({ userID: values.userID, name: values.name.trim() });
      form.reset();
      closeDialog('issueForMember');
      setSelectedApiKey(apiKey);
      setOneTimeKeyNotice(true);
      openDialog('view', apiKey);
    } catch {
      // Errors are surfaced by the mutation's error handler.
    }
  };

  return (
    <Dialog open={open} onOpenChange={(nextOpen) => !nextOpen && handleClose()}>
      <DialogContent className='sm:max-w-[520px]' ref={setDialogContent}>
        <DialogHeader>
          <DialogTitle>{t('apikeys.dialogs.issueForMember.title')}</DialogTitle>
          <DialogDescription>{t('apikeys.dialogs.issueForMember.description')}</DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className='space-y-4'>
            <FormField
              control={form.control}
              name='userID'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('apikeys.dialogs.issueForMember.memberLabel')}</FormLabel>
                  <FormControl>
                    <AutoCompleteSelect
                      selectedValue={field.value}
                      onSelectedValueChange={field.onChange}
                      items={memberOptions}
                      isLoading={isLoadingUsers}
                      placeholder={t('apikeys.dialogs.issueForMember.memberPlaceholder')}
                      emptyMessage={t('apikeys.dialogs.issueForMember.noMembers')}
                      portalContainer={dialogContent}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='name'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('apikeys.dialogs.fields.name.label')}</FormLabel>
                  <FormControl>
                    <Input placeholder={t('apikeys.dialogs.fields.name.placeholder')} {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <DialogFooter>
              <Button type='button' variant='outline' onClick={handleClose} disabled={issueKey.isPending}>
                {t('common.buttons.cancel')}
              </Button>
              <Button type='submit' disabled={issueKey.isPending}>
                {issueKey.isPending ? t('common.buttons.creating') : t('apikeys.dialogs.issueForMember.submit')}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
