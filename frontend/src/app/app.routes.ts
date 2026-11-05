import { Routes } from '@angular/router';
import { workspaceOpenGuard } from '@core/workspace-guard';

export const routes: Routes = [
  {
    path: '',
    pathMatch: 'full',
    loadComponent: () =>
      import('@features/workspace/workspace-picker').then(
        (module) => module.WorkspacePicker,
      ),
  },
  {
    path: 'workspace',
    canActivate: [workspaceOpenGuard],
    loadComponent: () =>
      import('@features/workspace-shell/workspace-shell').then(
        (module) => module.WorkspaceShell,
      ),
    children: [
      {
        path: '',
        loadComponent: () =>
          import('@features/workspace-shell/workspace-welcome').then(
            (module) => module.WorkspaceWelcome,
          ),
      },
      {
        path: 'request',
        children: [
          {
            path: '**',
            loadComponent: () =>
              import('@features/request-editor/request-editor').then(
                (module) => module.RequestEditor,
              ),
          },
        ],
      },
      {
        path: 'history',
        loadComponent: () =>
          import('@features/history/history-panel').then(
            (module) => module.HistoryPanel,
          ),
      },
      {
        path: 'git',
        loadComponent: () =>
          import('@features/git/git-panel').then((module) => module.GitPanel),
      },
    ],
  },
  { path: '**', redirectTo: '' },
];
