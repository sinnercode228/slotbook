import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import type { Customer } from '../api/types';
import type { Lang } from '../i18n/dictionary';
import { renderWith } from '../test/render';
import { emptyContact } from './contact';
import { ContactForm, type ContactFormProps } from './ContactForm';

function Harness(props: Partial<ContactFormProps> & { onSubmit: (c: Customer) => void }) {
  const [values, setValues] = useState(emptyContact);
  return <ContactForm values={values} onChange={setValues} {...props} />;
}

function setup(props: Partial<ContactFormProps> = {}, lang: Lang = 'en') {
  const onSubmit = vi.fn();
  renderWith(<Harness onSubmit={onSubmit} {...props} />, { lang });
  return { onSubmit, user: userEvent.setup() };
}

describe('ContactForm', () => {
  it('does not submit without a name and a phone, says why and focuses the first problem', async () => {
    const { onSubmit, user } = setup();
    await user.click(screen.getByRole('button', { name: 'Confirm booking' }));
    expect(onSubmit).not.toHaveBeenCalled();
    expect(screen.getByLabelText('Name')).toHaveAccessibleDescription('Please enter your name.');
    expect(screen.getByLabelText('Phone')).toHaveAccessibleDescription(/Please enter a phone number/);
    expect(screen.getByLabelText('Name')).toHaveFocus();
  });

  it('applies the server rules to the phone and the email', async () => {
    const { onSubmit, user } = setup();
    await user.type(screen.getByLabelText('Name'), 'Anna');
    await user.type(screen.getByLabelText('Phone'), '123-45');
    await user.type(screen.getByLabelText(/Email/), 'anna@');
    await user.click(screen.getByRole('button', { name: 'Confirm booking' }));
    expect(onSubmit).not.toHaveBeenCalled();
    expect(screen.getByLabelText('Phone')).toHaveAttribute('aria-invalid', 'true');
    expect(screen.getByText('A phone number has 7 to 15 digits.')).toBeInTheDocument();
    expect(screen.getByText('This does not look like an email address.')).toBeInTheDocument();
  });

  it('submits trimmed details and leaves out an empty email', async () => {
    const { onSubmit, user } = setup();
    await user.type(screen.getByLabelText('Name'), '  Anna Petrova ');
    await user.type(screen.getByLabelText('Phone'), ' +7 900 123-45-67');
    await user.click(screen.getByRole('button', { name: 'Confirm booking' }));
    expect(onSubmit).toHaveBeenCalledWith({ name: 'Anna Petrova', phone: '+7 900 123-45-67' });
  });

  it('waits until a field is left before complaining, then follows each keystroke', async () => {
    const { user } = setup();
    const phone = screen.getByLabelText('Phone');
    await user.type(phone, '12');
    expect(phone).toHaveAttribute('aria-invalid', 'false');
    await user.tab();
    expect(phone).toHaveAttribute('aria-invalid', 'true');
    await user.click(phone);
    await user.type(phone, '34567');
    expect(phone).toHaveAttribute('aria-invalid', 'false');
  });

  it('shows a problem the server found on its field', () => {
    setup({ serverProblem: { field: 'email', problem: 'email_invalid' } });
    expect(screen.getByLabelText(/Email/)).toHaveAccessibleDescription('This does not look like an email address.');
  });

  it('keeps the button busy while the booking is confirmed', () => {
    setup({ busy: true });
    expect(screen.getByRole('button', { name: /Confirming/ })).toBeDisabled();
  });

  it('speaks Russian when asked to', async () => {
    const { user } = setup({}, 'ru');
    await user.click(screen.getByRole('button', { name: 'Подтвердить запись' }));
    expect(screen.getByText('Укажите имя.')).toBeInTheDocument();
  });
});
