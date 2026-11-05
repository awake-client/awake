import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { WorkspaceStore } from '@core/workspace-store';

/** Redirects to the picker when no workspace is open. */
export const workspaceOpenGuard: CanActivateFn = () => {
  const workspaceStore = inject(WorkspaceStore);
  const router = inject(Router);

  return workspaceStore.isOpen() ? true : router.createUrlTree(['/']);
};
