import { Component, inject } from '@angular/core';
import { RouterOutlet } from '@angular/router';
import { suppressDefaultContextMenu } from '@core/context-menu';
import { ThemeStore } from '@core/theme-store';
import { WindowControls } from '@shared/window-controls';

@Component({
  imports: [RouterOutlet, WindowControls],
  selector: 'app-root',
  styleUrl: './app.css',
  templateUrl: './app.html',
})
export class App {
  // Created at startup so the saved theme applies before anything renders.
  private readonly theme = inject(ThemeStore);

  constructor() {
    suppressDefaultContextMenu();
  }
}
