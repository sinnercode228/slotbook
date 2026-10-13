import type { SVGProps } from 'react';

// Stroke icons drawn on a 20px grid; they take the current text colour.
function Icon({ children, ...props }: SVGProps<SVGSVGElement>) {
  return (
    <svg
      width="20"
      height="20"
      viewBox="0 0 20 20"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
      {...props}
    >
      {children}
    </svg>
  );
}

export const ChevronLeft = (p: SVGProps<SVGSVGElement>) => (
  <Icon {...p}>
    <path d="M12.5 4.5 7 10l5.5 5.5" />
  </Icon>
);

export const ChevronRight = (p: SVGProps<SVGSVGElement>) => (
  <Icon {...p}>
    <path d="M7.5 4.5 13 10l-5.5 5.5" />
  </Icon>
);

export const Check = (p: SVGProps<SVGSVGElement>) => (
  <Icon {...p}>
    <path d="m4.5 10.5 3.5 3.5 7.5-8" />
  </Icon>
);

export const Clock = (p: SVGProps<SVGSVGElement>) => (
  <Icon {...p}>
    <circle cx="10" cy="10" r="7" />
    <path d="M10 6.5V10l2.5 1.5" />
  </Icon>
);

export const MapPin = (p: SVGProps<SVGSVGElement>) => (
  <Icon {...p}>
    <path d="M10 17.5s5.5-4.8 5.5-9.25a5.5 5.5 0 0 0-11 0C4.5 12.7 10 17.5 10 17.5Z" />
    <circle cx="10" cy="8.25" r="2" />
  </Icon>
);

export const Users = (p: SVGProps<SVGSVGElement>) => (
  <Icon {...p}>
    <circle cx="7.5" cy="7" r="2.75" />
    <path d="M2.5 16c.5-2.75 2.5-4.25 5-4.25s4.5 1.5 5 4.25" />
    <path d="M13 4.6a2.75 2.75 0 0 1 0 4.8M14.5 11.9c1.6.5 2.7 1.9 3 4.1" />
  </Icon>
);

export const CalendarIcon = (p: SVGProps<SVGSVGElement>) => (
  <Icon {...p}>
    <rect x="3" y="4.5" width="14" height="12.5" rx="2" />
    <path d="M3 8.5h14M7 2.75v3M13 2.75v3" />
  </Icon>
);

export const CopyIcon = (p: SVGProps<SVGSVGElement>) => (
  <Icon {...p}>
    <rect x="7" y="7" width="10" height="10" rx="2" />
    <path d="M13 7V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v6a2 2 0 0 0 2 2h2" />
  </Icon>
);

export const AlertIcon = (p: SVGProps<SVGSVGElement>) => (
  <Icon {...p}>
    <circle cx="10" cy="10" r="7.25" />
    <path d="M10 6.25v4.5M10 13.6v.15" />
  </Icon>
);

export const CloseIcon = (p: SVGProps<SVGSVGElement>) => (
  <Icon {...p}>
    <path d="m5.5 5.5 9 9M14.5 5.5l-9 9" />
  </Icon>
);
