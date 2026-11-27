import { describe, expect, it } from 'vitest';
import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderApp } from '@/test/render';
import { db } from '@/test/gateway';

describe('вход, регистрация и гарды', () => {
  it('гость регистрируется и попадает в каталог уже зрителем', async () => {
    const user = userEvent.setup();
    renderApp('/register');
    const form = await screen.findByRole('form', { name: 'Регистрация' });
    await user.type(within(form).getByLabelText('Как к вам обращаться'), 'Вера');
    await user.type(within(form).getByLabelText('Email'), 'vera@example.com');
    await user.type(within(form).getByLabelText('Пароль'), 'password1');
    await user.click(within(form).getByRole('button', { name: 'Создать аккаунт' }));
    expect(await screen.findByRole('heading', { name: 'Кино, которое пережило век' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Вера' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Кабинет' })).toBeInTheDocument();
    expect(db.users.some((u) => u.email === 'vera@example.com' && u.role === 'viewer')).toBe(true);
  });

  it('занятый email подсвечивается у поля', async () => {
    const user = userEvent.setup();
    renderApp('/register');
    const form = await screen.findByRole('form', { name: 'Регистрация' });
    await user.type(within(form).getByLabelText('Как к вам обращаться'), 'Анна');
    await user.type(within(form).getByLabelText('Email'), 'anna@example.com');
    await user.type(within(form).getByLabelText('Пароль'), 'password1');
    await user.click(within(form).getByRole('button', { name: 'Создать аккаунт' }));
    expect(await within(form).findAllByText('Этот email уже зарегистрирован')).toHaveLength(2);
  });

  it('неверный пароль — сообщение, без входа', async () => {
    const user = userEvent.setup();
    renderApp('/login');
    await user.type(await screen.findByLabelText('Email'), 'anna@example.com');
    await user.type(screen.getByLabelText('Пароль'), 'wrong');
    await user.click(screen.getByRole('button', { name: 'Войти' }));
    expect(await screen.findByText('Неверный email или пароль')).toBeInTheDocument();
  });

  it('закрытая страница отправляет гостя на вход и возвращает после него', async () => {
    const user = userEvent.setup();
    renderApp('/cabinet');
    await user.type(await screen.findByLabelText('Email'), 'anna@example.com');
    await user.type(screen.getByLabelText('Пароль'), 'password1');
    await user.click(screen.getByRole('button', { name: 'Войти' }));
    expect(await screen.findByRole('heading', { name: 'Личный кабинет' })).toBeInTheDocument();
  });

  it('зрителю закрыта админка, админу — плеер', async () => {
    const { unmount } = renderApp('/admin/titles', { as: 'viewer' });
    expect(await screen.findByRole('heading', { name: 'Нет доступа' })).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Продажи' })).not.toBeInTheDocument();
    unmount();
    renderApp('/watch/t-metropolis', { as: 'admin' });
    expect(await screen.findByRole('heading', { name: 'Нет доступа' })).toBeInTheDocument();
  });

  it('несуществующий адрес — 404', async () => {
    renderApp('/no/such/page');
    expect(await screen.findByRole('heading', { name: '404' })).toBeInTheDocument();
  });

  it('выход возвращает гостевое меню', async () => {
    const user = userEvent.setup();
    renderApp('/', { as: 'viewer' });
    await user.click(await screen.findByRole('button', { name: 'Выйти' }));
    expect(await screen.findByRole('link', { name: 'Войти' })).toBeInTheDocument();
    expect(db.sessionDevice).toBeNull();
  });
});
