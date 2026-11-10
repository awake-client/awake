import { TestBed } from '@angular/core/testing';
import { SecretInput, referencesVariables } from './secret-input';

describe('referencesVariables', () => {
  it('recognises a value made of a variable', () => {
    expect(referencesVariables('{{password}}')).toBe(true);
    expect(referencesVariables('{{ $token }}')).toBe(true);
  });

  it('recognises a variable still being typed', () => {
    expect(referencesVariables('{{')).toBe(true);
    expect(referencesVariables('{{pass')).toBe(true);
  });

  it('recognises a variable inside a larger value', () => {
    expect(referencesVariables('Bearer {{token}}')).toBe(true);
  });

  it('leaves a literal secret alone', () => {
    expect(referencesVariables('s3cr3t')).toBe(false);
    expect(referencesVariables('')).toBe(false);
  });
});

describe('SecretInput', () => {
  beforeEach(async () => {
    await TestBed.configureTestingModule({ imports: [SecretInput] }).compileComponents();
  });

  function render(value: string): HTMLElement {
    const fixture = TestBed.createComponent(SecretInput);
    fixture.componentRef.setInput('value', value);
    fixture.detectChanges();
    return fixture.nativeElement as HTMLElement;
  }

  function field(element: HTMLElement): HTMLElement {
    return element.querySelector('app-variable-input')!;
  }

  function revealButton(element: HTMLElement): HTMLButtonElement | null {
    return element.querySelector('.reveal');
  }

  it('masks a literal secret and offers to reveal it', () => {
    const element = render('s3cr3t');

    expect(field(element).classList).toContain('masked');
    expect(revealButton(element)).not.toBeNull();
  });

  it('shows a value that references a variable', () => {
    const element = render('{{password}}');

    expect(field(element).classList).not.toContain('masked');
    expect(revealButton(element)).toBeNull();
  });

  it('shows a variable reference while it is typed', () => {
    const element = render('{{pass');

    expect(field(element).classList).not.toContain('masked');
  });

  it('shows a variable even after the user hid a literal secret', () => {
    const fixture = TestBed.createComponent(SecretInput);
    fixture.componentRef.setInput('value', 's3cr3t');
    fixture.detectChanges();

    const button = revealButton(fixture.nativeElement as HTMLElement)!;
    // Reveal the literal value, then hide it again.
    button.click();
    fixture.detectChanges();
    button.click();
    fixture.detectChanges();
    expect(field(fixture.nativeElement as HTMLElement).classList).toContain('masked');

    fixture.componentRef.setInput('value', '{{password}}');
    fixture.detectChanges();

    expect(field(fixture.nativeElement as HTMLElement).classList).not.toContain('masked');
  });

  it('reveals a literal secret on demand', () => {
    const fixture = TestBed.createComponent(SecretInput);
    fixture.componentRef.setInput('value', 's3cr3t');
    fixture.detectChanges();

    revealButton(fixture.nativeElement as HTMLElement)!.click();
    fixture.detectChanges();

    expect(field(fixture.nativeElement as HTMLElement).classList).not.toContain('masked');
  });
});
