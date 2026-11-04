import { Routes } from '@angular/router';

export const routes: Routes = [
  {
    path: '',
    loadComponent: () =>
      import('@features/workspace/workspace-picker').then(
        (module) => module.WorkspacePicker,
      ),
  },
];
