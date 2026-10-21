import * as React from 'react';

/**
 * Avatar — from poketeamcalculatorfrontend@0.0.0.
 */
export interface AvatarProps {
/** Nombre del Pokémon; se muestra su inicial. */ name: string; /** Diámetro en px (32 lista, 44–48 tarjeta, 96 destacado). */ size?: number; /** 'avatar-accent' = fondo naranja de acento. */ className?: string;
}

export declare const Avatar: React.ComponentType<AvatarProps>;
